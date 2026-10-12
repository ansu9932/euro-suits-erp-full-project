// email-preview creates offline examples; it never sends mail or reads credentials.
package main

import (
	_ "embed"
	"eurosuits/erp/internal/brandedmail"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

//go:embed index.html
var index []byte

func main() {
	dir := "../docs/email-previews"
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		panic(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), index, 0644); err != nil {
		panic(err)
	}
	today := time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)
	examples := map[string]brandedmail.Message{
		"welcome":        brandedmail.Welcome(brandedmail.WelcomeData{ID: 2, Name: "Aarav", Username: "example.employee", Password: "Illustration-only!", Designation: "Fabric Sourcing Manager", Department: "Sourcing", Company: "Euro Suits", Scope: "Assigned companies", Dashboard: "https://erp.eurosuitsmfg.in/sourcing", Responsibilities: []string{"View assigned orders", "Manage material samples and sourcing"}, Active: true}),
		"sign-in":        brandedmail.SignIn("048291"),
		"renewal":        brandedmail.Renewal("Example Garments Pvt. Ltd.", "Factory operating licence", today.AddDate(0, 0, 30), today, "https://erp.example.com"),
		"due-today":      brandedmail.Renewal("Example Garments Pvt. Ltd.", "Factory operating licence", today, today, "https://erp.example.com"),
		"overdue":        brandedmail.Renewal("Example Garments Pvt. Ltd.", "Factory operating licence", today.AddDate(0, 0, -12), today, "https://erp.example.com"),
		"access-blocked": brandedmail.Renewal("Example Garments Pvt. Ltd.", "Factory operating licence", today.AddDate(0, 0, -30), today, "https://erp.example.com"),
	}
	for name, message := range examples {
		page, err := brandedmail.Preview(message)
		if err != nil {
			panic(err)
		}
		if err = os.WriteFile(filepath.Join(dir, name+".html"), page, 0644); err != nil {
			panic(err)
		}
		eml, err := brandedmail.MIME("sender@example.com", "recipient@example.com", message)
		if err != nil {
			panic(err)
		}
		if err = os.WriteFile(filepath.Join(dir, name+".eml"), eml, 0644); err != nil {
			panic(err)
		}
	}
	fmt.Println("Created HTML and .eml previews with fictional company data in", dir)
}
