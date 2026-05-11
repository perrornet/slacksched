package acp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"unicode/utf8"
)

// maxACPTraceBytes caps one logged JSON line to avoid huge log lines.
const maxACPTraceBytes = 256 * 1024

// ServerHandler processes agent→client JSON-RPC requests (identified by method + id).
// Return result JSON (object) or set err to send a generic error response.
type ServerHandler func(ctx context.Context, id int64, method string, params json.RawMessage) (result json.RawMessage, err error)

// Client performs newline-delimited JSON-RPC on stdin/stdout of a provider.
type Client struct {
	in  io.Writer
	out *bufio.Scanner

	mu       sync.Mutex
	nextID   int64
	pending  map[int64]chan *RPCResponse
	onNotify func(method string, params json.RawMessage)
	server   ServerHandler
	log      *slog.Logger
	trace    bool

	readErr error
	wg      sync.WaitGroup
}

// ClientOption configures Client.
type ClientOption func(*Client)

// WithACPTrace logs every raw JSON-RPC line sent to or received from the provider (Info: acp_trace).
func WithACPTrace(log *slog.Logger, on bool) ClientOption {
	return func(c *Client) {
		c.log = log
		c.trace = on
	}
}

// NewClient starts a background reader on out (line-delimited JSON).
func NewClient(in io.Writer, out io.Reader, onNotify func(method string, params json.RawMessage), server ServerHandler, opts ...ClientOption) *Client {
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	c := &Client{
		in:       in,
		out:      sc,
		pending:  make(map[int64]chan *RPCResponse),
		onNotify: onNotify,
		server:   server,
	}
	for _, o := range opts {
		o(c)
	}
	c.wg.Add(1)
	go c.readLoop()
	return c
}

func (c *Client) logTrace(dir string, line []byte) {
	if !c.trace || c.log == nil || len(line) == 0 {
		return
	}
	s := truncateUTF8(string(line), maxACPTraceBytes)
	c.log.Info("acp_trace", "direction", dir, "raw", s)
}

func truncateUTF8(s string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(s) <= maxBytes {
		return s
	}
	s = s[:maxBytes]
	for len(s) > 0 && !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s + "…[truncated]"
}

func (c *Client) readLoop() {
	defer c.wg.Done()
	ctx := context.Background()
	for c.out.Scan() {
		line := bytes.TrimSpace(c.out.Bytes())
		if len(line) == 0 {
			continue
		}
		c.logTrace("recv", line)
		var meta struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if err := json.Unmarshal(line, &meta); err != nil {
			if c.trace && c.log != nil {
				c.log.Debug("acp_trace_parse_error", "direction", "recv", "err", err)
			}
			continue
		}
		hasID := len(meta.ID) > 0 && string(meta.ID) != "null"

		switch {
		case meta.Method != "" && !hasID:
			var note struct {
				Method string          `json:"method"`
				Params json.RawMessage `json:"params"`
			}
			_ = json.Unmarshal(line, &note)
			if c.onNotify != nil && note.Method != "" {
				c.onNotify(note.Method, note.Params)
			}

		case meta.Method != "" && hasID:
			var idVal int64
			_ = json.Unmarshal(meta.ID, &idVal)
			var req struct {
				Params json.RawMessage `json:"params"`
			}
			_ = json.Unmarshal(line, &req)
			c.handleServerRequest(ctx, idVal, meta.Method, req.Params)

		default:
			var resp RPCResponse
			if err := json.Unmarshal(line, &resp); err != nil {
				continue
			}
			c.mu.Lock()
			ch := c.pending[resp.ID]
			if ch != nil {
				delete(c.pending, resp.ID)
			}
			c.mu.Unlock()
			if ch != nil {
				select {
				case ch <- &resp:
				default:
				}
			}
		}
	}
	if err := c.out.Err(); err != nil {
		c.mu.Lock()
		c.readErr = err
		for id, ch := range c.pending {
			close(ch)
			delete(c.pending, id)
		}
		c.mu.Unlock()
	}
}

func (c *Client) handleServerRequest(ctx context.Context, id int64, method string, params json.RawMessage) {
	if c.server == nil {
		c.writeV(map[string]any{
			"jsonrpc": "2.0",
			"id":      id,
			"error": map[string]any{
				"code":    -32601,
				"message": "Method not found (no server handler): " + method,
			},
		})
		return
	}
	res, err := c.server(ctx, id, method, params)
	if err != nil {
		c.writeV(map[string]any{
			"jsonrpc": "2.0",
			"id":      id,
			"error": map[string]any{
				"code":    -32603,
				"message": err.Error(),
			},
		})
		return
	}
	if len(res) == 0 {
		res = json.RawMessage(`{}`)
	}
	_ = c.writeV(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  json.RawMessage(res),
	})
}

func (c *Client) writeV(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return c.sendRawLine(b)
}

func (c *Client) sendRawLine(b []byte) error {
	c.logTrace("send", b)
	_, err := fmt.Fprintf(c.in, "%s\n", b)
	return err
}

// Call sends a request and waits for the matching response.
func (c *Client) Call(ctx context.Context, method string, params any, result any) error {
	id := atomic.AddInt64(&c.nextID, 1)
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return err
	}
	req := RPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  paramsJSON,
	}
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	ch := make(chan *RPCResponse, 1)
	c.mu.Lock()
	if c.readErr != nil {
		err := c.readErr
		c.mu.Unlock()
		return err
	}
	c.pending[id] = ch
	c.mu.Unlock()

	if err := c.sendRawLine(body); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return err
	}

	for {
		select {
		case <-ctx.Done():
			c.mu.Lock()
			delete(c.pending, id)
			c.mu.Unlock()
			return ctx.Err()
		case resp, ok := <-ch:
			if !ok {
				return c.readErrOr(io.ErrUnexpectedEOF)
			}
			c.mu.Lock()
			delete(c.pending, id)
			c.mu.Unlock()
			if resp.Error != nil {
				return fmt.Errorf("rpc error %d: %s", resp.Error.Code, resp.Error.Message)
			}
			if result != nil && len(resp.Result) > 0 {
				if err := json.Unmarshal(resp.Result, result); err != nil {
					return fmt.Errorf("decode result: %w", err)
				}
			}
			return nil
		}
	}
}

func (c *Client) readErrOr(fallback error) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.readErr != nil {
		return c.readErr
	}
	return fallback
}

// WaitReadLoop waits for reader exit (e.g. after process dies).
func (c *Client) WaitReadLoop() {
	c.wg.Wait()
}

// ClosePending cancels waiters; used on shutdown.
func (c *Client) ClosePending() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, ch := range c.pending {
		close(ch)
		delete(c.pending, id)
	}
}

var errNoRead = errors.New("acp: no read error stored")

// ReadError returns scanner/pipe error after read loop stops.
func (c *Client) ReadError() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.readErr == nil {
		return errNoRead
	}
	return c.readErr
}
