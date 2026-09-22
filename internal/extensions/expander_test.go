package extensions_test

import (
	"reflect"
	"testing"

	"github.com/unsubble/searchit/internal/extensions"
)

func TestExpander(t *testing.T) {
	words := []string{"foo", "bar", "baz"}

	t.Run("no extensions", func(t *testing.T) {
		exp := extensions.NewExpander(nil)
		if exp.HasExtensions() {
			t.Errorf("expected HasExtensions=false")
		}
		if exp.VariantCount() != 1 {
			t.Errorf("expected VariantCount=1, got %d", exp.VariantCount())
		}
		if variants := exp.Variants("foo"); !reflect.DeepEqual(variants, []string{"foo"}) {
			t.Errorf("expected [foo], got %v", variants)
		}
		if eager := exp.ExpandEager(words); !reflect.DeepEqual(eager, words) {
			t.Errorf("expected %v, got %v", words, eager)
		}
		if levels := exp.Levels(words); !reflect.DeepEqual(levels, [][]string{words}) {
			t.Errorf("expected [%v], got %v", words, levels)
		}
	})

	t.Run("empty extension only", func(t *testing.T) {
		exp := extensions.NewExpander([]string{""})
		if !exp.HasExtensions() {
			t.Errorf("expected HasExtensions=true")
		}
		if exp.VariantCount() != 1 {
			t.Errorf("expected VariantCount=1, got %d", exp.VariantCount())
		}
		if eager := exp.ExpandEager(words); !reflect.DeepEqual(eager, words) {
			t.Errorf("expected %v, got %v", words, eager)
		}
	})

	t.Run("multiple extensions with empty prefix", func(t *testing.T) {
		exts, err := extensions.Parse([]string{",php,txt"})
		if err != nil {
			t.Fatalf("Parse failed: %v", err)
		}
		exp := extensions.NewExpander(exts)
		if exp.VariantCount() != 3 {
			t.Fatalf("expected VariantCount=3, got %d", exp.VariantCount())
		}

		expectedEager := []string{
			"foo", "foo.php", "foo.txt",
			"bar", "bar.php", "bar.txt",
			"baz", "baz.php", "baz.txt",
		}
		if eager := exp.ExpandEager(words); !reflect.DeepEqual(eager, expectedEager) {
			t.Errorf("expected eager %v, got %v", expectedEager, eager)
		}

		expectedLevels := [][]string{
			{"foo", "bar", "baz"},
			{"foo.php", "bar.php", "baz.php"},
			{"foo.txt", "bar.txt", "baz.txt"},
		}
		if levels := exp.Levels(words); !reflect.DeepEqual(levels, expectedLevels) {
			t.Errorf("expected levels %v, got %v", expectedLevels, levels)
		}
	})
}
