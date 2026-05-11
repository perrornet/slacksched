package workspace

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// CreateSessionWorkspace builds a unique directory under workspacesRoot and
// writes agent markdown from template plus appended system prompt.
// If slackMrkdwnGuideSrc is non-empty, that file is copied into the workspace
// as references/slack-mrkdwn-guide.md.
// sessionBot is written into AGENTS.md when UserID is non-empty (once per new workspace).
// BuildSchedulerAgentConstraintsMarkdown is written at the top of AGENTS.md (code-generated).
// When contextAPIBaseURL is non-empty, a Slack context HTTP API section is appended after the session bot block.
func CreateSessionWorkspace(workspacesRoot, teamID, channelID, rootThreadTS, uniqueSuffix, templatePath, agentFilename, appendPrompt, slackMrkdwnGuideSrc, contextAPIBaseURL string, sessionBot SessionBotIdentity) (string, error) {
	base, err := filepath.Abs(workspacesRoot)
	if err != nil {
		return "", fmt.Errorf("workspaces root: %w", err)
	}
	safeTS := sanitizePathSegment(rootThreadTS)
	dirName := fmt.Sprintf("%s-%s", safeTS, sanitizePathSegment(uniqueSuffix))
	full := filepath.Join(base, sanitizePathSegment(teamID), sanitizePathSegment(channelID), dirName)
	if err := os.MkdirAll(full, 0o755); err != nil {
		return "", fmt.Errorf("mkdir workspace: %w", err)
	}
	tplData, err := os.ReadFile(templatePath)
	if err != nil {
		return "", fmt.Errorf("read template: %w", err)
	}
	agentPath := filepath.Join(full, agentFilename)
	var b strings.Builder
	constraints := BuildSchedulerAgentConstraintsMarkdown(agentFilename, contextAPIBaseURL)
	if strings.TrimSpace(constraints) != "" {
		b.WriteString(constraints)
		if !strings.HasSuffix(constraints, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	b.Write(tplData)
	if appendPrompt != "" {
		if !strings.HasSuffix(b.String(), "\n") {
			b.WriteString("\n")
		}
		b.WriteString("\n")
		b.WriteString(appendPrompt)
		if !strings.HasSuffix(appendPrompt, "\n") {
			b.WriteString("\n")
		}
	}
	if sec := sessionBot.agentMarkdownSection(); sec != "" {
		b.WriteString(sec)
	}
	if doc := BuildAgentContextAPISectionMarkdown(contextAPIBaseURL); doc != "" {
		b.WriteString(doc)
	}
	if err := os.WriteFile(agentPath, []byte(b.String()), 0o644); err != nil {
		return "", fmt.Errorf("write agent md: %w", err)
	}
	guideSrc := strings.TrimSpace(slackMrkdwnGuideSrc)
	if guideSrc != "" {
		refDir := filepath.Join(full, "references")
		if err := os.MkdirAll(refDir, 0o755); err != nil {
			return "", fmt.Errorf("mkdir references: %w", err)
		}
		dst := filepath.Join(refDir, "slack-mrkdwn-guide.md")
		if err := copyFile(dst, guideSrc); err != nil {
			return "", fmt.Errorf("copy slack mrkdwn guide: %w", err)
		}
	}
	return full, nil
}

func copyFile(dst, src string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		_ = out.Close()
	}()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

func sanitizePathSegment(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "unknown"
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := b.String()
	if out == "" {
		return "segment"
	}
	return out
}

// RemoveAll deletes the workspace directory tree.
func RemoveAll(workspacePath string) error {
	if workspacePath == "" || workspacePath == "/" {
		return fmt.Errorf("refusing to remove unsafe path %q", workspacePath)
	}
	return os.RemoveAll(workspacePath)
}

// Archive moves workspace to archiveRoot preserving relative structure.
func Archive(workspacePath, archiveRoot string) error {
	if workspacePath == "" || workspacePath == "/" {
		return fmt.Errorf("refusing to archive unsafe path %q", workspacePath)
	}
	rel, err := filepath.Rel(filepath.Dir(workspacePath), workspacePath)
	if err != nil || strings.Contains(rel, "..") {
		rel = filepath.Base(workspacePath)
	}
	dest := filepath.Join(archiveRoot, rel)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	return os.Rename(workspacePath, dest)
}
