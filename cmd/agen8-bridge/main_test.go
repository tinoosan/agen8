package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestServeRequiresValidFlagParsing(t *testing.T) {
	cmd := newRootCommand()
	cmd.SetArgs([]string{"serve", "--http-addr", ""})
	err := cmd.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "http address is required")
}
