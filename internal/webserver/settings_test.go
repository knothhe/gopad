package webserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"
)

func TestSharedTranslationsAndUpdatePreview(t *testing.T) {
	h := handler(&mockCatalog{}, Controls{SystemLocale: func() string { return "zh-Hans-CN" }, Version: "0.1.0"})
	w := request(h, "GET", "/api/i18n", "", "")
	var got struct {
		Locale   string
		Language string
		Messages map[string]string
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusOK || got.Locale != "zh-CN" || got.Language != "system" || got.Messages["category.all"] != "全部" {
		t.Fatal(w.Body.String())
	}
	w = request(h, "GET", "/api/update", "", "")
	var state UpdateState
	if err := json.Unmarshal(w.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if state.Status != "preview" || state.CurrentVersion != "0.1.0" {
		t.Fatal(state)
	}
	w = request(h, "POST", "/api/update/check", "{}", "")
	if w.Code != http.StatusConflict {
		t.Fatal(w.Code)
	}
}

func TestUpdateActionsAreExplicitAndSameOrigin(t *testing.T) {
	checks := 0
	automatic := true
	fail := false
	h := handler(&mockCatalog{}, Controls{
		UpdateStatus: func() UpdateState { return UpdateState{Status: "ready", AutomaticChecks: automatic} },
		CheckUpdates: func() { checks++ },
		SetAutomaticChecks: func(value bool) error {
			if fail {
				return errors.New("disk full")
			}
			automatic = value
			return nil
		},
	})
	if w := request(h, "GET", "/api/update/check", "", ""); w.Code != http.StatusMethodNotAllowed {
		t.Fatal(w.Code)
	}
	if w := request(h, "POST", "/api/update/check", "{}", "https://other.example"); w.Code != http.StatusForbidden {
		t.Fatal(w.Code)
	}
	if checks != 0 {
		t.Fatal("unauthorized update action")
	}
	if w := request(h, "POST", "/api/update/check", "{}", ""); w.Code != http.StatusOK || checks != 1 {
		t.Fatal(w.Code, checks)
	}
	if w := request(h, "PUT", "/api/update/preferences", `{"automaticChecks":false}`, ""); w.Code != http.StatusOK || automatic {
		t.Fatal(w.Code, automatic)
	}
	fail = true
	if w := request(h, "PUT", "/api/update/preferences", `{"automaticChecks":true}`, ""); w.Code != http.StatusInternalServerError || automatic {
		t.Fatal(w.Code, automatic)
	}
	if w := request(h, "PUT", "/api/update/preferences", `{}`, ""); w.Code != http.StatusBadRequest {
		t.Fatal(w.Code)
	}
}
