package app

import (
	"reflect"
	"testing"
)

func TestBusinessFlowsInput(t *testing.T) {
	current := M{"business_flows": []string{"Domestic", "Import", "Export"}, "business_mode": "Import & export"}
	for _, tc := range []struct {
		name    string
		input   M
		current M
		want    []string
	}{
		{"new unspecified", M{}, nil, []string{}},
		{"legacy combined", M{"business_mode": "Import & export"}, nil, []string{"Import", "Export"}},
		{"legacy domestic", M{"business_mode": "Domestic"}, nil, []string{"Domestic"}},
		{"canonical deduplicated", M{"business_flows": []any{"Export", "Domestic", "Import", "Domestic"}}, nil, []string{"Domestic", "Import", "Export"}},
		{"omitted preserves", M{}, current, []string{"Domestic", "Import", "Export"}},
		{"legacy echo preserves", M{"business_mode": "Import & export"}, current, []string{"Domestic", "Import", "Export"}},
		{"changed legacy accepted", M{"business_mode": "Domestic"}, current, []string{"Domestic"}},
		{"explicit clear", M{"business_flows": []any{}}, current, []string{}},
		{"array is authoritative", M{"business_flows": []any{"Export"}, "business_mode": "Domestic"}, current, []string{"Export"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := businessFlowsInput(tc.input, tc.current)
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, %v; want %v", got, err, tc.want)
			}
		})
	}
	for _, input := range []M{
		{"business_flows": nil}, {"business_flows": "Domestic"}, {"business_flows": M{}},
		{"business_flows": []any{"Not specified"}}, {"business_flows": []any{"Import & export"}},
		{"business_flows": []any{"domestic"}}, {"business_flows": []any{" Import"}},
		{"business_flows": []any{true}}, {"business_flows": []any{nil}},
		{"business_mode": nil}, {"business_mode": 3}, {"business_mode": "Bad"},
		{"business_flows": []any{"Domestic"}, "business_mode": "Bad"},
	} {
		if _, err := businessFlowsInput(input, current); err == nil {
			t.Errorf("accepted invalid input %#v", input)
		}
	}
}

func TestCompanyBusinessFlowFiltersAndCompatibility(t *testing.T) {
	for _, tc := range []struct {
		flows   []string
		mode    string
		filters []string
		label   string
	}{
		{[]string{}, "Not specified", []string{"Not specified"}, "Not specified"},
		{[]string{"Domestic"}, "Domestic", []string{"Domestic"}, "Domestic"},
		{[]string{"Import"}, "Import", []string{"Import"}, "Import"},
		{[]string{"Export"}, "Export", []string{"Export"}, "Export"},
		{[]string{"Domestic", "Import"}, "Import", []string{"Domestic", "Import"}, "Domestic, Import"},
		{[]string{"Domestic", "Export"}, "Export", []string{"Domestic", "Export"}, "Domestic, Export"},
		{[]string{"Import", "Export"}, "Import & export", []string{"Import", "Export", "Import & export"}, "Import, Export"},
		{[]string{"Domestic", "Import", "Export"}, "Import & export", []string{"Domestic", "Import", "Export", "Import & export"}, "Domestic, Import, Export"},
	} {
		c := M{"business_flows": tc.flows, "business_mode": tc.mode}
		if got := businessModeForFlows(tc.flows); got != tc.mode {
			t.Errorf("mode %v: got %s", tc.flows, got)
		}
		if got := companyBusinessFlowFilters(c); !reflect.DeepEqual(got, tc.filters) {
			t.Errorf("filters %v: got %v", tc.flows, got)
		}
		if got := companyBusinessFlowLabel(c); got != tc.label {
			t.Errorf("label %v: got %s", tc.flows, got)
		}
	}
	if got := companyBusinessFlows(M{"business_mode": "Import & export"}); !reflect.DeepEqual(got, []string{"Import", "Export"}) {
		t.Errorf("legacy row fallback: %v", got)
	}
}
