package resume

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ledongthuc/pdf"
)

// ExtractText extracts plain text from a document byte slice.
// Supported extensions: .pdf, .docx, .doc
func ExtractText(data []byte, ext string) (string, error) {
	switch ext {
	case ".pdf":
		return extractPDF(data)
	case ".docx":
		return extractDOCX(data)
	case ".doc":
		return extractDOC(data)
	default:
		return "", fmt.Errorf("unsupported format: %s", ext)
	}
}

// extractPDF uses ledongthuc/pdf to extract text from a PDF.
func extractPDF(data []byte) (string, error) {
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("pdf open: %w", err)
	}

	var buf strings.Builder
	for i := 1; i <= r.NumPage(); i++ {
		page := r.Page(i)
		if page.V.IsNull() {
			continue
		}
		text, err := page.GetPlainText(nil)
		if err != nil {
			continue
		}
		buf.WriteString(text)
		buf.WriteString("\n")
	}
	return normalizeExtractedText(buf.String()), nil
}

// extractDOCX converts a .docx file to markdown using pandoc.
func extractDOCX(data []byte) (string, error) {
	tmpDir, err := os.MkdirTemp("", "docx-extract-*")
	if err != nil {
		return "", fmt.Errorf("docx: create temp dir: %w", err)
	}
	defer removeTempDir(tmpDir)

	inputPath := filepath.Join(tmpDir, "input.docx")
	if err := os.WriteFile(inputPath, data, 0o644); err != nil {
		return "", fmt.Errorf("docx: write temp file: %w", err)
	}

	return extractDOCXFile(inputPath)
}

func extractDOCXFile(path string) (string, error) {
	cmd := exec.Command("pandoc",
		"--from", "docx",
		"--to", "gfm",
		"--wrap=none",
		path,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docx: pandoc convert failed: %w\noutput: %s", err, output)
	}

	text := normalizeExtractedText(string(output))
	if text == "" {
		return "", fmt.Errorf("docx: pandoc produced empty output")
	}
	return text, nil
}

// extractDOC converts a .doc file to .docx using LibreOffice headless,
// then extracts text from the resulting .docx.
func extractDOC(data []byte) (string, error) {
	tmpDir, err := os.MkdirTemp("", "doc-convert-*")
	if err != nil {
		return "", fmt.Errorf("doc: create temp dir: %w", err)
	}
	defer removeTempDir(tmpDir)

	inputPath := filepath.Join(tmpDir, "input.doc")
	if err := os.WriteFile(inputPath, data, 0o644); err != nil {
		return "", fmt.Errorf("doc: write temp file: %w", err)
	}

	// Convert .doc to .docx using LibreOffice headless
	cmd := exec.Command("soffice",
		"--headless",
		"--convert-to", "docx",
		"--outdir", tmpDir,
		inputPath,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("doc: libreoffice convert failed: %w\noutput: %s", err, output)
	}

	outputPath := filepath.Join(tmpDir, "input.docx")
	if _, err := os.Stat(outputPath); err != nil {
		return "", fmt.Errorf("doc: converted docx missing: %w", err)
	}

	return extractDOCXFile(outputPath)
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
