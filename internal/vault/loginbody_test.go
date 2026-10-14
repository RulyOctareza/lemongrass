package vault

import (
	"strings"
	"testing"
)

func TestLoginBodyTypesFields(t *testing.T) {
	u := DomainUser{
		Fields:     map[string]string{"username": "alice", "site_id": " 3 ", "remember": "true", "ratio": "-1.5e2"},
		FieldTypes: map[string]string{"site_id": FieldNumber, "remember": FieldBoolean, "ratio": FieldNumber},
	}
	got, err := u.LoginBody()
	if err != nil {
		t.Fatalf("LoginBody: %v", err)
	}
	want := `{"ratio":-1.5e2,"remember":true,"site_id":3,"username":"alice"}`
	if string(got) != want {
		t.Errorf("LoginBody() = %s, want %s", got, want)
	}
}

func TestLoginBodyWithoutFieldTypesIsAllText(t *testing.T) {
	got, err := DomainUser{Fields: map[string]string{"site_id": "3"}}.LoginBody()
	if err != nil {
		t.Fatalf("LoginBody: %v", err)
	}
	if string(got) != `{"site_id":"3"}` {
		t.Errorf("LoginBody() = %s, want site_id as a string", got)
	}
}

func TestLoginBodyRejectsBadValues(t *testing.T) {
	cases := []struct {
		value, kind, want string
	}{
		{"abc", FieldNumber, "is not a number"},
		{"", FieldNumber, "is not a number"},
		{"[1]", FieldNumber, "is not a number"},
		{"0x10", FieldNumber, "is not a number"},
		{"yes", FieldBoolean, "is not true or false"},
		{"3", "date", `has an unknown type "date"`},
	}
	for _, c := range cases {
		u := DomainUser{Fields: map[string]string{"f": c.value}, FieldTypes: map[string]string{"f": c.kind}}
		if _, err := u.LoginBody(); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("LoginBody(%q as %q) = %v, want an error containing %q", c.value, c.kind, err, c.want)
		}
	}
}

func TestPutDomainRejectsBadTypedField(t *testing.T) {
	svc, err := NewService(t.TempDir())
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	d := Domain{BaseURL: "https://api.example.test", LoginEndpoint: "/login", Users: []DomainUser{{
		Name:       "alice",
		Fields:     map[string]string{"site_id": "three"},
		FieldTypes: map[string]string{"site_id": FieldNumber},
	}}}
	err = svc.PutDomain("root-secret", "staging", d)
	if err == nil || err.Error() != `vault: user "alice": login field "site_id" is not a number` {
		t.Errorf("PutDomain(bad number) = %v, want the user and field named", err)
	}

	d.Users[0].Fields["site_id"] = "3"
	if err := svc.PutDomain("root-secret", "staging", d); err != nil {
		t.Errorf("PutDomain(valid number) = %v, want nil", err)
	}
	got, err := svc.getDomain("root-secret", "staging")
	if err != nil {
		t.Fatalf("getDomain: %v", err)
	}
	if got.Users[0].FieldTypes["site_id"] != FieldNumber {
		t.Errorf("stored FieldTypes = %v, want site_id as number", got.Users[0].FieldTypes)
	}
}
