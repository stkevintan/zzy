package resume

import (
	"os"
	"os/exec"
	"testing"
)

func requireCommand(t *testing.T, name string) {
	t.Helper()
	if _, err := exec.LookPath(name); err != nil {
		t.Skipf("%s not installed: %v", name, err)
	}
}

func TestExtractPDF(t *testing.T) {
	requireCommand(t, "markitdown")

	data, err := os.ReadFile("../samples/何晓娇地理教师简历.pdf")
	if err != nil {
		t.Skip("sample PDF not found:", err)
	}
	text, err := ExtractText(data, ".pdf")
	if err != nil {
		t.Fatalf("ExtractText PDF: %v", err)
	}
	if len(text) == 0 {
		t.Fatal("extracted empty text from PDF")
	}
	t.Logf("PDF text (%d chars):\n%.500s", len(text), text)
}

func TestExtractDOC(t *testing.T) {
	requireCommand(t, "markitdown")

	data, err := os.ReadFile("../samples/任喆烜 语文.doc")
	if err != nil {
		t.Skip("sample DOC not found:", err)
	}
	text, err := ExtractText(data, ".doc")
	if err != nil {
		t.Fatalf("ExtractText DOC: %v", err)
	}
	if len(text) == 0 {
		t.Fatal("extracted empty text from DOC")
	}
	t.Logf("DOC text (%d chars):\n%.500s", len(text), text)
}
