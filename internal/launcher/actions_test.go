package launcher

import (
	"reflect"
	"testing"
)

func TestRepeatedFavoriteAdditionIsIdempotentAndToggleStillRemoves(t *testing.T) {
	s := testService(t)
	for range 2 {
		if _, err := s.ApplyAppAction("favorite-add", "a", ""); err != nil {
			t.Fatal(err)
		}
	}
	if got := s.Preferences().Favorites; !reflect.DeepEqual(got, []string{"a"}) {
		t.Fatalf("repeated favorite drops removed or duplicated app: %v", got)
	}
	reopened, err := New(s.configPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.Preferences().Favorites; !reflect.DeepEqual(got, []string{"a"}) {
		t.Fatalf("favorite additions were not saved: %v", got)
	}
	if _, err := s.ApplyAppAction("favorite", "a", ""); err != nil {
		t.Fatal(err)
	}
	if len(s.Preferences().Favorites) != 0 {
		t.Fatal("context-menu toggle no longer removes a favorite")
	}
}
