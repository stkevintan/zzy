package resume

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
	"unicode/utf16"

	"github.com/ledongthuc/pdf"
	"github.com/richardlehane/mscfb"
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

// extractDOC extracts text from an old binary .doc file using OLE2/CFB parsing.
// It reads the WordDocument stream and extracts UTF-16LE encoded text.
func extractDOC(data []byte) (string, error) {
	doc, err := mscfb.New(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("doc cfb open: %w", err)
	}

	for entry, err := doc.Next(); err == nil; entry, err = doc.Next() {
		name := entry.Name
		if name == "WordDocument" {
			return extractWordDocText(entry)
		}
	}

	// Fallback: try to find any text stream
	return "", fmt.Errorf("doc: WordDocument stream not found")
}

// extractWordDocText attempts to extract readable text from the WordDocument stream.
// The .doc binary format stores text as UTF-16LE in the text portion of the stream.
func extractWordDocText(r io.Reader) (string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}

	// The FIB (File Information Block) starts at offset 0.
	// We need at least the first few fields to find the text.
	if len(data) < 24 {
		return "", fmt.Errorf("doc: WordDocument stream too short")
	}

	// Try to extract text directly from the stream as UTF-16LE sequences.
	// This is a best-effort approach for the binary .doc format.
	return extractUTF16Text(data), nil
}

// extractUTF16Text scans binary data for UTF-16LE encoded text sequences.
func extractUTF16Text(data []byte) string {
	var buf strings.Builder
	var u16buf []uint16

	// Scan for UTF-16LE characters (common in .doc text streams)
	for i := 0; i+1 < len(data); i += 2 {
		ch := binary.LittleEndian.Uint16(data[i : i+2])

		// Printable character ranges (ASCII + CJK + common punctuation)
		if isPrintableUTF16(ch) {
			u16buf = append(u16buf, ch)
		} else {
			if len(u16buf) >= 4 { // only keep sequences of 4+ chars
				runes := utf16.Decode(u16buf)
				buf.WriteString(string(runes))
				buf.WriteString("\n")
			}
			u16buf = u16buf[:0]
		}
	}
	if len(u16buf) >= 4 {
		runes := utf16.Decode(u16buf)
		buf.WriteString(string(runes))
	}

	return strings.TrimSpace(buf.String())
}

func isPrintableUTF16(ch uint16) bool {
	// ASCII printable + whitespace
	if ch >= 0x20 && ch <= 0x7E {
		return true
	}
	if ch == '\t' || ch == '\n' || ch == '\r' {
		return true
	}
	// CJK Unified Ideographs
	if ch >= 0x4E00 && ch <= 0x9FFF {
		return true
	}
	// CJK punctuation and symbols
	if ch >= 0x3000 && ch <= 0x303F {
		return true
	}
	// Fullwidth forms (fullwidth ASCII, etc.)
	if ch >= 0xFF00 && ch <= 0xFFEF {
		return true
	}
	// CJK Compatibility Ideographs
	if ch >= 0xF900 && ch <= 0xFAFF {
		return true
	}
	// Hangul, Katakana, Hiragana (for completeness)
	if ch >= 0x3040 && ch <= 0x30FF {
		return true
	}
	// General CJK range
	if ch >= 0x2E80 && ch <= 0x2FFF {
		return true
	}
	return false
}
