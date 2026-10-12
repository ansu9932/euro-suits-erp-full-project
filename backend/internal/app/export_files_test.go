package app

import (
	"bytes"
	"image"
	"image/jpeg"
	"image/png"
	"testing"
)

func TestExportDocumentPresentation(t *testing.T) {
	var pngBytes, jpegBytes bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	if err := png.Encode(&pngBytes, img); err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(&jpegBytes, img, nil); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, filename, disposition, mime string
		body                              []byte
		preview                           bool
	}{
		{"PDF preview", "invoice.pdf", "inline", "application/pdf", []byte("%PDF-1.7\n%%EOF"), true},
		{"PNG preview", "scan.png", "inline", "image/png", pngBytes.Bytes(), true},
		{"JPEG preview", "scan.JPEG", "inline", "image/jpeg", jpegBytes.Bytes(), true},
		{"PDF download", "invoice.pdf", "attachment", "application/octet-stream", []byte("%PDF-1.7"), false},
		{"image download", "scan.png", "attachment", "application/octet-stream", pngBytes.Bytes(), false},
		{"fake PDF", "invoice.pdf", "attachment", "application/octet-stream", []byte("<html><script>alert(1)</script></html>"), true},
		{"fake image", "scan.png", "attachment", "application/octet-stream", []byte("%PDF-1.7"), true},
		{"wrong raster extension", "scan.jpg", "attachment", "application/octet-stream", pngBytes.Bytes(), true},
		{"HTML cannot preview", "invoice.html", "attachment", "application/octet-stream", []byte("%PDF-1.7"), true},
		{"SVG cannot preview", "invoice.svg", "attachment", "application/octet-stream", []byte("<svg></svg>"), true},
		{"Office download fallback", "invoice.docx", "attachment", "application/octet-stream", []byte("PK original Office bytes"), true},
		{"spreadsheet download fallback", "invoice.xlsx", "attachment", "application/octet-stream", []byte("PK original Office bytes"), true},
		{"archive download fallback", "invoice.zip", "attachment", "application/octet-stream", []byte("PK original ZIP bytes"), true},
		{"plain text download fallback", "invoice.txt", "attachment", "application/octet-stream", []byte("private invoice text"), true},
		{"unsafe filename", "invoice\r\n.pdf", "attachment", "application/octet-stream", []byte("%PDF-1.7"), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := append([]byte(nil), tc.body...)
			disposition, contentType := exportDocumentPresentation(tc.filename, tc.body, tc.preview)
			if disposition != tc.disposition || contentType != tc.mime {
				t.Fatalf("got %q %q; want %q %q", disposition, contentType, tc.disposition, tc.mime)
			}
			if !bytes.Equal(original, tc.body) {
				t.Fatal("document presentation changed original bytes")
			}
		})
	}
}
