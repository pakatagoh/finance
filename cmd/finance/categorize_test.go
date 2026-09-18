package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestCategorizeRejectsIncompleteAndUnknownArgumentsBeforeLoadingConfig(t *testing.T) {
	for _, args := range [][]string{{"--limit"}, {"--other", "1"}, {"--limit", "1", "extra"}} {
		var out, errOut bytes.Buffer
		err := categorizeCommand(context.Background(), args, &out, &errOut)
		if err == nil || !strings.Contains(err.Error(), "usage: finance categorize [--limit N]") {
			t.Fatalf("args=%v error=%v", args, err)
		}
	}
}
