package app

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestExportReconciliationPreservesCertificateFields(t *testing.T) {
	original := M{"ebrc_number": "OLD-1", "ebrc_amount": "400.00", "currency": "USD", "ebrc_file_number": "FILE-9", "ebrc_note": "Keep this note", "statuses": M{"ebrc_amount": "Verified"}}
	parent := M{"ebrcs": []M{original, {"ebrc_number": "OTHER-2", "shipping_bill_number": "SB-7"}}}
	originalJSON := exportJSON(parent)
	out, changed, err := exportReconciliationEBRCValues(parent, M{"version": 7, "previous_ebrc_number": " old-1 ", "ebrc_number": "CORRECT-3"})
	if err != nil || !changed {
		t.Fatalf("correction failed: %v", err)
	}
	compare := materialProfile(out[0])
	compare["ebrc_number"] = "OLD-1"
	if !reflect.DeepEqual(compare, original) || !reflect.DeepEqual(out[1], enquiryRows(parent["ebrcs"])[1]) {
		t.Fatal("correction changed other certificate fields")
	}
	if exportJSON(parent) != originalJSON {
		t.Fatal("correction mutated caller data")
	}
	added, changed, err := exportReconciliationEBRCValues(parent, M{"ebrc_number": "NEW-4"})
	if err != nil || !changed || len(added) != 3 || str(added[2], "ebrc_number") != "NEW-4" {
		t.Fatal("eBRC append failed")
	}
	for _, body := range []M{{"ebrc_number": "old-1"}, {"ebrc_number": "OLD-1", "previous_ebrc_number": "OLD-1"}} {
		got, changed, err := exportReconciliationEBRCValues(parent, body)
		if err != nil || changed || !reflect.DeepEqual(got, enquiryRows(parent["ebrcs"])) {
			t.Fatal("duplicate unchanged evidence must be a no-op")
		}
	}
}

func TestExportReconciliationRejectsMixedAndInvalidEvidence(t *testing.T) {
	parent := M{"ebrcs": []M{{"ebrc_number": "ONE"}, {"ebrc_number": "TWO"}}}
	for _, body := range []M{
		{"ebrc_number": "NEW", "invoice_amount": "0.00"}, {"ebrc_number": "NEW", "workflow_status": "Approved"}, {"ebrc_number": "NEW", "ebrc_amount": "0"},
		{"ebrc_number": "ONE", "previous_ebrc_number": "TWO"}, {"ebrc_number": "NEW", "previous_ebrc_number": "MISSING"},
		{"ebrc_number": "NEW", "previous_ebrc_number": nil}, {"ebrc_number": "NEW", "previous_ebrc_number": ""},
		{"ebrc_number": nil}, {"ebrc_number": true}, {"ebrc_number": ""}, {"ebrc_number": "X\nY"}, {"ebrc_number": strings.Repeat("x", 161)},
	} {
		if _, _, err := exportReconciliationEBRCValues(parent, body); err == nil {
			t.Fatalf("accepted invalid reconciliation input: %v", body)
		}
	}
	hundred := []M{}
	for i := 0; i < 100; i++ {
		hundred = append(hundred, M{"ebrc_number": fmt.Sprintf("E-%d", i)})
	}
	if _, _, err := exportReconciliationEBRCValues(M{"ebrcs": hundred}, M{"ebrc_number": "NEW"}); err == nil {
		t.Fatal("eBRC capacity exceeded")
	}
	if _, changed, err := exportReconciliationEBRCValues(M{"ebrcs": hundred}, M{"previous_ebrc_number": "E-99", "ebrc_number": "CORRECT"}); err != nil || !changed {
		t.Fatal("capacity should still allow correcting an existing number")
	}
}

func TestExportReconciliationAccessIndependentOfApproval(t *testing.T) {
	for _, status := range []string{"Draft", "Submitted", "Approved", "Changes requested"} {
		parent := M{"record_kind": "export", "workflow_status": status, "invoice_amount": "1000.00", "approved_discount": "0.00"}
		for _, role := range []string{"Finance", "Finance Head", "Super Admin"} {
			u := &User{Role: role, Scope: "company"}
			for _, files := range []bool{false, true} {
				if err := exportReconciliationAccess(u, parent, files); err != nil {
					t.Fatalf("authorized %s blocked in %s: %v", role, status, err)
				}
			}
			a := exportWorkflowActions(u, parent)
			if a["reconciliation_evidence"] != true || a["reconciliation_attach"] != true || a["reconciliation_remove"] != true {
				t.Fatal("reconciliation actions missing")
			}
			if status == "Approved" || status == "Submitted" {
				if a["attach"] != false || a["edit"] != false || a["remove"] != false {
					t.Fatal("normal invoice freeze weakened")
				}
			}
		}
		for _, role := range []string{"MD", "Admin", "Merchandiser", "Developer"} {
			if exportReconciliationAccess(&User{Role: role}, parent, false) == nil {
				t.Fatalf("noneditor %s allowed", role)
			}
		}
		nofiles := &User{Role: "Finance", CustomPermissions: true, Permissions: []string{"export_view", "export_edit"}}
		if exportReconciliationAccess(nofiles, parent, false) != nil || exportReconciliationAccess(nofiles, parent, true) == nil {
			t.Fatal("file permission boundary lost")
		}
		noedit := &User{Role: "Finance", CustomPermissions: true, Permissions: []string{"export_view", "export_files"}}
		if exportReconciliationAccess(noedit, parent, true) == nil {
			t.Fatal("file grant alone allowed reconciliation edits")
		}
	}
	if exportReconciliationAccess(&User{Role: "Super Admin"}, M{"record_kind": "jv"}, false) == nil {
		t.Fatal("JV accepted reconciliation evidence")
	}
}
