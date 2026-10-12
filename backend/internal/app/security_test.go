package app

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestUntrustedJSON(t *testing.T) {
	for _, input := range []string{`{"role":"Finance","role":"Super Admin"}`, `{"profile":{"value":"a","value":"b"}}`, `{"a":` + strings.Repeat(`[`, 18) + `0` + strings.Repeat(`]`, 18) + `}`, `{"a":"` + strings.Repeat("x", 20001) + `"}`, `{"a":"\u0000"}`, `{"a":1} {"b":2}`, `[]`, `null`, `{"session_version":9}`} {
		r := httptest.NewRequest("POST", "/", strings.NewReader(input))
		if _, err := readBody(httptest.NewRecorder(), r); err == nil {
			t.Errorf("accepted invalid request %.70s", input)
		}
	}
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{"profile":{"name":"A & B <text>","enabled":true},"id":7}`))
	if _, err := readBody(httptest.NewRecorder(), r); err != nil {
		t.Fatal(err)
	}
}
func TestUploadValidation(t *testing.T) {
	var pngBytes bytes.Buffer
	png.Encode(&pngBytes, image.NewRGBA(image.Rect(0, 0, 2, 2)))
	archive := func(names ...string) []byte {
		var b bytes.Buffer
		z := zip.NewWriter(&b)
		for _, name := range names {
			f, _ := z.Create(name)
			f.Write([]byte("test"))
		}
		z.Close()
		return b.Bytes()
	}
	cases := []struct {
		name string
		data []byte
		ok   bool
	}{
		{"proof.pdf", []byte("%PDF-1.4\nproof"), true}, {"proof.pdf", []byte("<script>alert(1)</script>"), false},
		{"image.png", pngBytes.Bytes(), true}, {"image.jpg", pngBytes.Bytes(), false}, {"image.png", []byte("MZ executable"), false},
		{"book.xlsx", archive("[Content_Types].xml", "xl/workbook.xml"), true},
		{"book.xlsx", archive("readme.txt"), false}, {"book.xlsx", archive("[Content_Types].xml", "xl/workbook.xml", "xl/vbaProject.bin"), false},
		{"archive.zip", archive("../escape.txt"), false}, {"archive.zip", archive("script.js"), false}, {"archive.zip", archive("a.txt", "a.txt"), false},
		{"archive.zip", archive("proof.txt"), true}, {"nested.zip", archive("more.zip"), false},
		{"proof.html", []byte("hello"), false}, {"proof.svg", []byte("<svg/>"), false}, {"proof\n.pdf", []byte("%PDF-"), false},
		{"proof.txt", []byte("MZ\x00"), false}, {"proof.txt", []byte("Normal text"), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := validateUpload(c.name, c.data) == nil; got != c.ok {
				t.Errorf("accepted=%v want %v", got, c.ok)
			}
		})
	}
}
func TestBotVerificationClaims(t *testing.T) {
	now := time.Now().UTC()
	good := botResult{true, "erp.example.test", "erp-login", now.Format(time.RFC3339)}
	if !validBotResult(good, "erp.example.test", now) {
		t.Fatal("rejected valid claims")
	}
	for _, change := range []func(*botResult){func(r *botResult) { r.Success = false }, func(r *botResult) { r.Hostname = "attacker.test" }, func(r *botResult) { r.Action = "other-form" }, func(r *botResult) { r.ChallengeTS = now.Add(-301 * time.Second).Format(time.RFC3339) }, func(r *botResult) { r.ChallengeTS = now.Add(2 * time.Minute).Format(time.RFC3339) }} {
		bad := good
		change(&bad)
		if validBotResult(bad, "erp.example.test", now) {
			t.Fatal("accepted invalid claims")
		}
	}
}
func TestProxyHeadersAndMediaType(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "127.0.0.1:1234"
	r.Header.Set("X-Real-IP", "1.2.3.4")
	t.Setenv("TRUST_PROXY", "")
	if clientIP(r) != "127.0.0.1" {
		t.Fatal("trusted an unconfigured proxy")
	}
	t.Setenv("TRUST_PROXY", "true")
	if clientIP(r) != "1.2.3.4" {
		t.Fatal("did not use trusted proxy")
	}
	r.Header.Set("X-Real-IP", "malformed")
	if clientIP(r) != "127.0.0.1" {
		t.Fatal("accepted malformed address")
	}
	if requestMediaType("application/json-evil") == "application/json" {
		t.Fatal("accepted wrong media type")
	}
	if requestMediaType("application/json; charset=utf-8") != "application/json" {
		t.Fatal("rejected valid media type")
	}
}
func TestKYCAuthenticatedEncryption(t *testing.T) {
	t.Setenv("KYC_ENCRYPTION_KEY", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")
	encrypted, e := sealCompany(CompanyPrivate{Owner: "Confidential"})
	if e != nil {
		t.Fatal(e)
	}
	encrypted[len(encrypted)-1] ^= 1
	if _, e = openCompany(M{"private_data": encrypted}); e == nil {
		t.Fatal("tampered ciphertext was accepted")
	}
	raw, _ := json.Marshal(CompanyPrivate{Owner: "Confidential"})
	if bytes.Contains(encrypted, raw) {
		t.Fatal("plaintext leaked")
	}
}
