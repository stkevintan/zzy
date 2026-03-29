package resume

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ExtractText extracts plain text from a document byte slice.
// Supported extensions: .pdf, .docx, .doc
func ExtractText(data []byte, ext string) (string, error) {
	switch ext {
	case ".pdf", ".docx", ".doc":
		return extractWithMarkItDown(data, ext)
	default:
		return "", fmt.Errorf("unsupported format: %s", ext)
	}
}

// extractWithMarkItDown uses the markitdown CLI to convert a file to markdown.
func extractWithMarkItDown(data []byte, ext string) (string, error) {
	tmpDir, err := os.MkdirTemp("", "markitdown-*")
	if err != nil {
		return "", fmt.Errorf("markitdown: create temp dir: %w", err)
	}
	defer removeTempDir(tmpDir)

	inputPath := filepath.Join(tmpDir, "input"+ext)
	if err := os.WriteFile(inputPath, data, 0o644); err != nil {
		return "", fmt.Errorf("markitdown: write temp file: %w", err)
	}

	cmd := exec.Command("markitdown", inputPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("markitdown failed: %w\noutput: %s", err, output)
	}

	text := normalizeExtractedText(string(output))
	if text == "" {
		return "", fmt.Errorf("markitdown produced empty output")
	}
	return text, nil
}

func normalizeExtractedText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	for strings.Contains(text, "\n\n\n") {
		text = strings.ReplaceAll(text, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(text)
}

func removeTempDir(path string) {
	if err := os.RemoveAll(path); err != nil {
		slog.Warn("failed to remove temp dir", "path", path, "error", err)
	}
}
