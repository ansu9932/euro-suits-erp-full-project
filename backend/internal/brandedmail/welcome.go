package brandedmail

import (
	"fmt"
	"net/url"
	"strings"
)

type WelcomeData struct {
	ID                                                                           int64
	Name, Username, Password, Designation, Department, Company, Scope, Dashboard string
	Responsibilities                                                             []string
	Active                                                                       bool
}

func Welcome(d WelcomeData) Message {
	encouragement := []string{
		"Your care and consistency help every team deliver work we can be proud of. Bring your best to each detail, and let progress grow one good decision at a time.",
		"Great garments are built by people who take ownership. Your ideas, accuracy and teamwork can make a lasting difference here.",
		"Every well-planned task and every carefully checked detail moves the whole company forward. Your contribution matters from day one.",
		"Excellence grows through steady effort and shared learning. Ask questions, support your colleagues and take pride in the quality you bring.",
		"Behind every finished garment is someone who chose to do the work well. Let your commitment set a positive example for the team.",
		"Start with confidence, work with purpose and keep improving. Your thoughtful actions today help build a stronger team tomorrow.",
	}
	index := d.ID % int64(len(encouragement))
	if index < 0 {
		index = -index
	}
	m := Message{
		Subject: "Welcome to Euro Suits — your ERP account", Preheader: "Your role, responsibilities and private sign-in details are ready.",
		Category: "WELCOME TO THE TEAM", Title: "Welcome, " + d.Name + ".",
		Intro:  "Your administrator has created your Euro Suits ERP account. Below are the responsibilities and access assigned to you.",
		Status: "YOUR WORKSPACE IS READY", ActionLabel: "Open your dashboard", Welcome: true,
		Note:             "These sign-in details are private. Do not forward this email or share your password. Contact your administrator for account changes or help with your access.",
		Encouragement:    fmt.Sprintf("%s, as %s, your contribution is valued. %s", d.Name, d.Designation, encouragement[index]),
		Responsibilities: d.Responsibilities,
		Details:          []Detail{{"Assigned role", d.Designation}, {"Department", d.Department}, {"Company assignment", d.Company}, {"Record access", d.Scope}, {"User ID", d.Username}, {"Initial password", d.Password}},
	}
	if !d.Active {
		m.Status = "ACCOUNT CREATED · AWAITING ACTIVATION"
		m.Intro = "Your administrator has prepared your Euro Suits ERP account. Sign-in will become available when your administrator activates it."
	}
	if u, e := url.Parse(d.Dashboard); e == nil && u.Scheme == "https" && u.Host != "" && u.User == nil && !strings.ContainsAny(d.Dashboard, "\r\n") {
		m.ActionURL = d.Dashboard
	}
	m.Text = "EURO SUITS | WELCOME\n\n" + m.Title + "\n" + m.Intro + "\n" + m.Status + "\n"
	for _, item := range m.Details {
		m.Text += "\n" + item.Label + ": " + item.Value
	}
	m.Text += "\n\nYour assigned responsibilities:\n"
	for _, item := range m.Responsibilities {
		m.Text += "- " + item + "\n"
	}
	m.Text += "\n" + m.Encouragement + "\n\nOpen your dashboard: " + m.ActionURL + "\n\n" + m.Note
	return m
}
