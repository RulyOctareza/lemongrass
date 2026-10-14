package gatekeeper

import (
	"errors"
	"testing"

	"github.com/faizalv/lemongrass/internal/restergate"
)

func TestTestDomainLoginResultCarriesStatusAndBody(t *testing.T) {
	got := testDomainLoginResult(&restergate.LoginStatusError{Status: 422, Body: []byte(`{"detail":"bad"}`)})
	want := testDomainLoginResultPayload{Error: "restergate: login returned status 422", Status: 422, Body: `{"detail":"bad"}`}
	if got != want {
		t.Errorf("testDomainLoginResult(status error) = %+v, want %+v", got, want)
	}
}

func TestTestDomainLoginResultOtherErrorHasNoBody(t *testing.T) {
	got := testDomainLoginResult(errors.New("restergate: logging in: connection refused"))
	if got.Error == "" || got.Status != 0 || got.Body != "" {
		t.Errorf("testDomainLoginResult(plain error) = %+v, want the message only", got)
	}
	if got := testDomainLoginResult(nil); got != (testDomainLoginResultPayload{}) {
		t.Errorf("testDomainLoginResult(nil) = %+v, want empty", got)
	}
}
