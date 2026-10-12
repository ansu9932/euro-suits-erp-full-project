package app

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWhatsAppSignature(t *testing.T) {
	b := []byte(`{"entry":[]}`)
	m := hmac.New(sha256.New, []byte("secret"))
	m.Write(b)
	s := "sha256=" + hex.EncodeToString(m.Sum(nil))
	if !waSignatureValid(b, s, "secret") {
		t.Fatal("valid rejected")
	}
	if waSignatureValid([]byte("tamper"), s, "secret") || waSignatureValid(b, s, "") || waSignatureValid(b, "bad", "secret") {
		t.Fatal("forgery accepted")
	}
}
func TestWhatsAppTemplate(t *testing.T) {
	valid := "Hello {{1}}, work {{2}}, details {{3}}, open {{4}}. Euro Suits ERP notification."
	if !waBodyValid(valid) {
		t.Fatal("valid rejected")
	}
	for _, s := range []string{"Hello {{1}}", "{{1}} work {{2}} details {{3}} link {{4}}", valid + " {{1}}", strings.Replace(valid, "{{2}}", "{{5}}", 1)} {
		if waBodyValid(s) {
			t.Fatal("invalid accepted")
		}
	}
}
func TestWhatsAppWebhookRejectsForgedRequest(t *testing.T) {
	t.Setenv("WHATSAPP_APP_SECRET", "secret")
	a := &App{}
	r := httptest.NewRequest("POST", "/api/whatsapp/webhook", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	a.whatsappWebhook(w, r)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
}
func TestWhatsAppWebhookChallenge(t *testing.T) {
	t.Setenv("WHATSAPP_VERIFY_TOKEN", "abc")
	a := &App{}
	r := httptest.NewRequest("GET", "/api/whatsapp/webhook?hub.mode=subscribe&hub.verify_token=abc&hub.challenge=123", nil)
	w := httptest.NewRecorder()
	a.whatsappWebhook(w, r)
	if w.Code != 200 || w.Body.String() != "123" {
		t.Fatal(w.Code, w.Body.String())
	}
}
