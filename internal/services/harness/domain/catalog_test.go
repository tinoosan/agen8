package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tinoosan/agen8-mcp-server/internal/services/harness/domain"
)

func testCatalog() *domain.Catalog {
	return domain.NewCatalog(
		domain.HarnessEntry{
			Kind: "claude-cli",
			Models: []domain.ModelEntry{
				{ID: "claude-opus-4-7", Efforts: []string{"low", "medium", "high", "xhigh", "max"}},
				{ID: "claude-sonnet-4-6", Efforts: []string{"low", "medium", "high", "xhigh", "max"}},
			},
		},
		domain.HarnessEntry{
			Kind: "codex",
			Models: []domain.ModelEntry{
				{ID: "gpt-5.5", Efforts: []string{"low", "medium", "high", "xhigh"}},
				{ID: "gpt-5.4", Efforts: []string{"low", "medium", "high", "xhigh"}, Aliases: []string{"gpt-5.1"}},
			},
		},
	)
}

func TestValidateConfig_Valid(t *testing.T) {
	c := testCatalog()
	require.NoError(t, c.ValidateConfig("claude-cli", "claude-opus-4-7", "high"))
	require.NoError(t, c.ValidateConfig("codex", "gpt-5.5", "low"))
}

func TestValidateConfig_PerHarnessEfforts(t *testing.T) {
	c := testCatalog()
	require.NoError(t, c.ValidateConfig("claude-cli", "claude-opus-4-7", "xhigh"))
	require.NoError(t, c.ValidateConfig("claude-cli", "claude-opus-4-7", "max"))
	require.NoError(t, c.ValidateConfig("codex", "gpt-5.5", "xhigh"))

	err := c.ValidateConfig("codex", "gpt-5.5", "max")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported effort")
}

func TestValidateConfig_Alias(t *testing.T) {
	c := testCatalog()
	require.NoError(t, c.ValidateConfig("codex", "gpt-5.1", "medium"))
}

func TestValidateConfig_UnsupportedHarness(t *testing.T) {
	c := testCatalog()
	err := c.ValidateConfig("unknown-harness", "some-model", "high")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported kind")
}

func TestValidateConfig_UnsupportedModel(t *testing.T) {
	c := testCatalog()
	err := c.ValidateConfig("claude-cli", "gpt-5.5", "high")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported model")
	assert.Contains(t, err.Error(), "claude-cli")
}

func TestValidateConfig_UnsupportedEffort(t *testing.T) {
	c := testCatalog()
	err := c.ValidateConfig("codex", "gpt-5.5", "max")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported effort")
}

func TestValidateConfig_EmptyFields(t *testing.T) {
	c := testCatalog()
	assert.ErrorContains(t, c.ValidateConfig("", "model", "high"), "kind is required")
	assert.ErrorContains(t, c.ValidateConfig("claude-cli", "", "high"), "model is required")
	assert.ErrorContains(t, c.ValidateConfig("claude-cli", "claude-opus-4-7", ""), "effort is required")
}

func TestValidateConfig_CaseInsensitive(t *testing.T) {
	c := testCatalog()
	require.NoError(t, c.ValidateConfig("Claude-CLI", "Claude-Opus-4-7", "High"))
}

func TestDefaultCatalog_ClaudeCliValid(t *testing.T) {
	c := domain.DefaultCatalog()
	require.NoError(t, c.ValidateConfig("claude-cli", "claude-opus-4-8", "high"))
	require.NoError(t, c.ValidateConfig("claude-cli", "claude-opus-4-7", "high"))
	require.NoError(t, c.ValidateConfig("claude-cli", "claude-sonnet-4-6", "medium"))
	require.NoError(t, c.ValidateConfig("claude-cli", "claude-opus-4-8", "xhigh"))
	require.NoError(t, c.ValidateConfig("claude-cli", "claude-opus-4-8", "max"))
}

func TestDefaultCatalog_CodexValid(t *testing.T) {
	c := domain.DefaultCatalog()
	require.NoError(t, c.ValidateConfig("codex", "gpt-5.5", "high"))
	require.NoError(t, c.ValidateConfig("codex", "gpt-5.5", "xhigh"))
	require.NoError(t, c.ValidateConfig("codex", "gpt-5.4", "low"))
}

func TestDefaultCatalog_DefaultPermissionModeUsesLoosestMode(t *testing.T) {
	c := domain.DefaultCatalog()
	assert.Equal(t, "codex/full-access", c.DefaultPermissionMode("codex"))
	assert.Equal(t, "claude-code/bypass-permissions", c.DefaultPermissionMode("claude-cli"))
}

func TestDefaultCatalog_DefaultPermissionFlagUsesLoosestMode(t *testing.T) {
	c := domain.DefaultCatalog()
	codexModes := c.PermissionModes("codex")
	require.NotEmpty(t, codexModes)
	var codexDefault string
	for _, mode := range codexModes {
		if mode.Default {
			codexDefault = mode.ID
		}
	}
	assert.Equal(t, "codex/full-access", codexDefault)

	claudeModes := c.PermissionModes("claude-cli")
	require.NotEmpty(t, claudeModes)
	var claudeDefault string
	for _, mode := range claudeModes {
		if mode.Default {
			claudeDefault = mode.ID
		}
	}
	assert.Equal(t, "claude-code/bypass-permissions", claudeDefault)
}

func TestDefaultCatalog_CodexRejectsMaxEffort(t *testing.T) {
	c := domain.DefaultCatalog()
	err := c.ValidateConfig("codex", "gpt-5.5", "max")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported effort")
}

func TestDefaultCatalog_CrossHarnessRejected(t *testing.T) {
	c := domain.DefaultCatalog()
	err := c.ValidateConfig("claude-cli", "gpt-5.5", "high")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported model")
}

func TestSupportedHarnesses(t *testing.T) {
	c := testCatalog()
	harnesses := c.SupportedHarnesses()
	assert.Len(t, harnesses, 2)
	assert.Contains(t, harnesses, "claude-cli")
	assert.Contains(t, harnesses, "codex")
}

func TestSupportedModels(t *testing.T) {
	c := testCatalog()
	models := c.SupportedModels("claude-cli")
	assert.Equal(t, []string{"claude-opus-4-7", "claude-sonnet-4-6"}, models)
}

func TestSupportedEfforts(t *testing.T) {
	c := testCatalog()
	efforts := c.SupportedEfforts("claude-cli", "claude-opus-4-7")
	assert.Equal(t, []string{"low", "medium", "high", "xhigh", "max"}, efforts)

	efforts = c.SupportedEfforts("codex", "gpt-5.5")
	assert.Equal(t, []string{"low", "medium", "high", "xhigh"}, efforts)

	efforts = c.SupportedEfforts("codex", "nonexistent")
	assert.Nil(t, efforts)
}
