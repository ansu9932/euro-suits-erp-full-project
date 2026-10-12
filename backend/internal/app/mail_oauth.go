package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/smtp"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

// Microsoft 365 client credentials are server-only. The token URL and scope
// are fixed; provider response bodies and access tokens are never logged.
func microsoftSMTPToken() (string, error) {
	tenant := os.Getenv("SMTP_TENANT_ID")
	client := os.Getenv("SMTP_CLIENT_ID")
	secret := os.Getenv("SMTP_CLIENT_SECRET")
	guid := regexp.MustCompile(`^[a-fA-F0-9-]{36}$`)
	if !guid.MatchString(tenant) || !guid.MatchString(client) || secret == "" {
		return "", fmt.Errorf("Microsoft 365 application credentials are not configured")
	}
	form := url.Values{"client_id": {client}, "client_secret": {secret}, "grant_type": {"client_credentials"}, "scope": {"https://outlook.office365.com/.default"}}
	c := &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, e := c.PostForm("https://login.microsoftonline.com/"+tenant+"/oauth2/v2.0/token", form)
	if e != nil {
		return "", fmt.Errorf("Microsoft 365 token request failed")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return "", fmt.Errorf("Microsoft 365 rejected application authentication")
	}
	var result struct {
		AccessToken string `json:"access_token"`
	}
	if e = json.NewDecoder(io.LimitReader(response.Body, 65536)).Decode(&result); e != nil || result.AccessToken == "" {
		return "", fmt.Errorf("Microsoft 365 token response was invalid")
	}
	return result.AccessToken, nil
}

type xoauthSMTP struct{ user, token, host string }

func (x xoauthSMTP) Start(server *smtp.ServerInfo) (string, []byte, error) {
	if !server.TLS || server.Name != x.host {
		return "", nil, fmt.Errorf("SMTP authentication requires the configured TLS server")
	}
	return "XOAUTH2", []byte("user=" + x.user + "\x01auth=Bearer " + x.token + "\x01\x01"), nil
}
func (x xoauthSMTP) Next(_ []byte, more bool) ([]byte, error) {
	if more {
		return nil, fmt.Errorf("SMTP OAuth authentication was rejected")
	}
	return nil, nil
}
func smtpAuthenticate(c *smtp.Client, host string) error {
	user := os.Getenv("SMTP_USERNAME")
	switch os.Getenv("SMTP_AUTH_MODE") {
	case "microsoft365":
		if !validEmail(user) {
			return fmt.Errorf("set the Microsoft 365 sender mailbox")
		}
		token, e := microsoftSMTPToken()
		if e != nil {
			return e
		}
		return c.Auth(xoauthSMTP{user, token, host})
	case "", "password":
		if user != "" {
			return c.Auth(smtp.PlainAuth("", user, os.Getenv("SMTP_PASSWORD"), host))
		}
		return nil
	default:
		return fmt.Errorf("unsupported SMTP authentication mode")
	}
}
func smtpCredentialsConfigured() bool {
	switch os.Getenv("SMTP_AUTH_MODE") {
	case "microsoft365":
		return validEmail(os.Getenv("SMTP_USERNAME")) && os.Getenv("SMTP_TENANT_ID") != "" && os.Getenv("SMTP_CLIENT_ID") != "" && os.Getenv("SMTP_CLIENT_SECRET") != ""
	case "", "password":
		return strings.TrimSpace(os.Getenv("SMTP_USERNAME")) == "" || os.Getenv("SMTP_PASSWORD") != ""
	default:
		return false
	}
}
