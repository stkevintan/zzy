package resume

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode/utf16"

	"github.com/richardlehane/mscfb"
)

// ExtractText extracts plain text from a document byte slice.
// Supported extensions: .pdf, .docx, .doc
func ExtractText(data []byte, ext string) (string, error) {
	switch ext {
	case ".pdf", ".docx":
		return extractWithMarkItDown(data, ext)
	case ".doc":
		return extractDOC(data)
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

// extractDOC extracts text from a .doc (Word Binary Format) file using pure Go.
func extractDOC(data []byte) (string, error) {
	doc, err := mscfb.New(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("doc: open compound file: %w", err)
	}

	streams := make(map[string][]byte)
	for entry, err := doc.Next(); err == nil; entry, err = doc.Next() {
		switch entry.Name {
		case "WordDocument", "0Table", "1Table":
			b, readErr := io.ReadAll(entry)
			if readErr != nil {
				return "", fmt.Errorf("doc: read %q stream: %w", entry.Name, readErr)
			}
			streams[entry.Name] = b
		}
	}

	wordDoc := streams["WordDocument"]
	if len(wordDoc) < 0x22 {
		return "", fmt.Errorf("doc: WordDocument stream missing or too short")
	}
	if binary.LittleEndian.Uint16(wordDoc[0:2]) != 0xA5EC {
		return "", fmt.Errorf("doc: invalid magic number")
	}

	// Bit 9 at offset 0x0A selects table stream
	tableName := "0Table"
	if binary.LittleEndian.Uint16(wordDoc[0x0A:0x0C])&0x0200 != 0 {
		tableName = "1Table"
	}
	tableStream := streams[tableName]
	if tableStream == nil {
		return "", fmt.Errorf("doc: %s stream not found", tableName)
	}

	// Walk FIB variable-length sections to reach fibRgFcLcb
	csw := int(binary.LittleEndian.Uint16(wordDoc[0x20:0x22]))
	pos := 0x22 + csw*2 // skip fibRgW
	if pos+2 > len(wordDoc) {
		return "", fmt.Errorf("doc: FIB truncated")
	}
	cslw := int(binary.LittleEndian.Uint16(wordDoc[pos : pos+2]))
	pos += 2 + cslw*4 // skip fibRgLw
	if pos+2 > len(wordDoc) {
		return "", fmt.Errorf("doc: FIB truncated")
	}
	cbRgFcLcb := int(binary.LittleEndian.Uint16(wordDoc[pos : pos+2]))
	pos += 2 // start of fibRgFcLcb pairs

	// fcClx/lcbClx is the 67th pair (index 66)
	const clxIdx = 66
	if cbRgFcLcb <= clxIdx {
		return "", fmt.Errorf("doc: FIB too old (%d FC/LCB pairs)", cbRgFcLcb)
	}
	off := pos + clxIdx*8
	if off+8 > len(wordDoc) {
		return "", fmt.Errorf("doc: FIB truncated at CLX offset")
	}
	fcClx := int(binary.LittleEndian.Uint32(wordDoc[off : off+4]))
	lcbClx := int(binary.LittleEndian.Uint32(wordDoc[off+4 : off+8]))
	if fcClx+lcbClx > len(tableStream) {
		return "", fmt.Errorf("doc: CLX extends beyond table stream")
	}

	text, err := docParseCLX(wordDoc, tableStream[fcClx:fcClx+lcbClx])
	if err != nil {
		return "", err
	}
	return normalizeExtractedText(text), nil
}

// docParseCLX parses the CLX structure to find and read the piece table.
func docParseCLX(wordDoc, clx []byte) (string, error) {
	i := 0
	for i < len(clx) {
		switch clx[i] {
		case 0x01: // Prc — skip
			if i+3 > len(clx) {
				return "", fmt.Errorf("doc: truncated Prc")
			}
			i += 3 + int(binary.LittleEndian.Uint16(clx[i+1:i+3]))
		case 0x02: // Pcdt
			if i+5 > len(clx) {
				return "", fmt.Errorf("doc: truncated Pcdt header")
			}
			lcb := int(binary.LittleEndian.Uint32(clx[i+1 : i+5]))
			plcPcd := clx[i+5:]
			if lcb > len(plcPcd) {
				return "", fmt.Errorf("doc: PlcPcd extends beyond CLX")
			}
			return docReadPieceTable(wordDoc, plcPcd[:lcb])
		default:
			return "", fmt.Errorf("doc: unexpected CLX entry type %#x", clx[i])
		}
	}
	return "", fmt.Errorf("doc: piece table not found")
}

// docReadPieceTable reconstructs text from the PlcPcd (piece table).
func docReadPieceTable(wordDoc, plcPcd []byte) (string, error) {
	// PlcPcd: (n+1) CPs (uint32) then n PCDs (8 bytes each)
	// Total = (n+1)*4 + n*8 → n = (len - 4) / 12
	n := (len(plcPcd) - 4) / 12
	if n < 1 {
		return "", fmt.Errorf("doc: empty piece table")
	}

	var buf strings.Builder
	for i := range n {
		cpStart := int(binary.LittleEndian.Uint32(plcPcd[i*4 : i*4+4]))
		cpEnd := int(binary.LittleEndian.Uint32(plcPcd[i*4+4 : i*4+8]))
		count := cpEnd - cpStart
		if count <= 0 {
			continue
		}

		pcdOff := (n+1)*4 + i*8
		if pcdOff+6 > len(plcPcd) {
			break
		}
		fcField := binary.LittleEndian.Uint32(plcPcd[pcdOff+2 : pcdOff+6])
		compressed := fcField&(1<<30) != 0
		fc := int(fcField &^ (1 << 30))

		if compressed {
			start := fc / 2
			if start+count > len(wordDoc) {
				continue
			}
			buf.Write(wordDoc[start : start+count])
		} else {
			byteLen := count * 2
			if fc+byteLen > len(wordDoc) {
				continue
			}
			u16 := make([]uint16, count)
			for j := range count {
				u16[j] = binary.LittleEndian.Uint16(wordDoc[fc+j*2 : fc+j*2+2])
			}
			buf.WriteString(string(utf16.Decode(u16)))
		}
	}

	// Translate Word control characters
	return strings.Map(func(r rune) rune {
		switch r {
		case 0x0D:
			return '\n'
		case 0x07, 0x09:
			return '\t'
		case 0x0B, 0x0C:
			return '\n'
		}
		if r < 0x20 && r != '\t' && r != '\n' {
			return -1
		}
		return r
	}, buf.String()), nil
}
