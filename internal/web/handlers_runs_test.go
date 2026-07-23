package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func formReq(body string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/run", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return r
}

const dateLayout = "2006/01/02"

func TestResolveBlastDatesLookbackDefault(t *testing.T) {
	// No date_mode => look-back mode; no months => default 12.
	from, to := resolveBlastDates(formReq(""))
	now := time.Now()
	if want := now.Format(dateLayout); to != want {
		t.Errorf("to = %q, want %q", to, want)
	}
	if want := now.AddDate(0, -12, 0).Format(dateLayout); from != want {
		t.Errorf("from = %q, want %q", from, want)
	}
}

func TestResolveBlastDatesLookbackClamp(t *testing.T) {
	now := time.Now()
	cases := map[string]int{
		"lookback_months=6":   6,
		"lookback_months=500": 240, // capped
		"lookback_months=0":   1,   // floored
		"lookback_months=-3":  1,
		"lookback_months=abc": 12, // unparseable => default
	}
	for body, months := range cases {
		from, _ := resolveBlastDates(formReq(body))
		if want := now.AddDate(0, -months, 0).Format(dateLayout); from != want {
			t.Errorf("body %q: from = %q, want %q (months %d)", body, from, want, months)
		}
	}
}

func TestResolveBlastDatesCustom(t *testing.T) {
	from, to := resolveBlastDates(formReq("date_mode=custom&blast_from=2020-01-01&blast_to=2024-12-31"))
	if from != "2020/01/01" || to != "2024/12/31" {
		t.Errorf("custom range: from=%q to=%q", from, to)
	}
}
