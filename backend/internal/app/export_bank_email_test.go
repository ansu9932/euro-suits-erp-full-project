package app

import (
	"bytes"
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"strings"
	"testing"
)

func TestBankMailInputAndMIME(t *testing.T) {
	good := func() M {
		return M{"version": 1, "recipients": []any{"bank@example.test"}, "subject": "Conversion advice – invoice 123", "body": "Please process the attached instruction.\nUSD 1,000.25", "file_ids": []any{4, 2}}
	}
	clean, e := exportBankMailInput(good())
	if e != nil {
		t.Fatal(e)
	}
	if ids := clean["file_ids"].([]int64); ids[0] != 2 || ids[1] != 4 {
		t.Fatal("attachment order not canonical")
	}
	for _, change := range []M{{"subject": "Hi\r\nBcc: other@example.test"}, {"recipients": []any{"Bank <bank@example.test>"}}, {"recipients": []any{"bad\r\n@example.test"}}, {"file_ids": []any{1, 1}}, {"file_ids": []any{}}, {"recipients": []any{}}, {"body": ""}, {"file_ids": []any{-1}}} {
		d := good()
		for k, v := range change {
			d[k] = v
		}
		if _, e := exportBankMailInput(d); e == nil {
			t.Fatalf("accepted invalid input: %v", change)
		}
	}
	rawFile := bytes.Repeat([]byte{0, 1, 128, 255, 42}, 31)
	raw, e := exportBankMIME("erp@example.test", []string{"bank@example.test"}, clean["subject"].(string), clean["body"].(string), []exportMailAttachment{{Name: "invoice été.pdf", Data: rawFile}}, 17)
	if e != nil {
		t.Fatal(e)
	}
	message, e := mail.ReadMessage(bytes.NewReader(raw))
	if e != nil {
		t.Fatal(e)
	}
	decoded, e := new(mime.WordDecoder).DecodeHeader(message.Header.Get("Subject"))
	if e != nil || decoded != clean["subject"] {
		t.Fatal("subject changed")
	}
	_, params, e := mime.ParseMediaType(message.Header.Get("Content-Type"))
	if e != nil {
		t.Fatal(e)
	}
	parts := multipart.NewReader(message.Body, params["boundary"])
	p, e := parts.NextRawPart()
	if e != nil {
		t.Fatal(e)
	}
	body, e := io.ReadAll(quotedprintable.NewReader(p))
	if e != nil || strings.ReplaceAll(string(body), "\r\n", "\n") != clean["body"] {
		t.Fatal("body changed")
	}
	p, e = parts.NextRawPart()
	if e != nil {
		t.Fatal(e)
	}
	_, params, e = mime.ParseMediaType(p.Header.Get("Content-Disposition"))
	if e != nil || params["filename"] != "invoice été.pdf" {
		t.Fatal("filename changed")
	}
	decodedFile, e := io.ReadAll(base64.NewDecoder(base64.StdEncoding, p))
	if e != nil || !bytes.Equal(decodedFile, rawFile) {
		t.Fatal("attachment bytes changed")
	}
	if _, e = parts.NextPart(); e != io.EOF {
		t.Fatal("unexpected extra MIME part")
	}
}
