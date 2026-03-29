package resume

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
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
	return strings.TrimSpace(buf.String()), nil
}

// extractDOCX reads a .docx file (ZIP containing XML) and extracts text.
func extractDOCX(data []byte) (string, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("docx zip open: %w", err)
	}

	for _, f := range r.File {
		if f.Name == "word/document.xml" {
			rc, err := f.Open()
			if err != nil {
				return "", fmt.Errorf("docx open document.xml: %w", err)
			}
			defer rc.Close()
			return parseWordXML(rc)
		}
	}
	return "", fmt.Errorf("docx: word/document.xml not found")
}

// parseWordXML extracts text from Word's document.xml.
func parseWordXML(r io.Reader) (string, error) {
	decoder := xml.NewDecoder(r)
	var buf strings.Builder
	var inText bool

	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			// <w:t> contains the actual text
			if t.Name.Local == "t" {
				inText = true
			}
		case xml.EndElement:
			if t.Name.Local == "t" {
				inText = false
			}
			// <w:p> marks a paragraph boundary
			if t.Name.Local == "p" {
				buf.WriteString("\n")
			}
		case xml.CharData:
			if inText {
				buf.Write(t)
			}
		}
	}
	return strings.TrimSpace(buf.String()), nil
}

// extractDOC converts a .doc file to .docx using LibreOffice headless,
// then extracts text from the resulting .docx.
func extractDOC(data []byte) (string, error) {
	tmpDir, err := os.MkdirTemp("", "doc-convert-*")
	if err != nil {
		return "", fmt.Errorf("doc: create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

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
	docxData, err := os.ReadFile(outputPath)
	if err != nil {
		return "", fmt.Errorf("doc: read converted docx: %w", err)
	}

	return extractDOCX(docxData)
}
