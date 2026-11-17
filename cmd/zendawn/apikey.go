package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"syscall"

	"golang.org/x/term"

	"github.com/kasra-sh/zendawn-go/config"
)

// resolveAPIKeyEnv picks the env var name for the default provider key.
func resolveAPIKeyEnv(doc *config.Document, provName string, agent *config.Agent) string {
	if doc != nil && provName != "" {
		if pc, ok := doc.Providers[provName]; ok && pc.APIKeyEnv != "" {
			return pc.APIKeyEnv
		}
	}
	if agent != nil && agent.Model.APIKeyEnv != "" {
		return agent.Model.APIKeyEnv
	}
	return "OPENAI_API_KEY"
}

// resolveAPIKey returns the first non-empty key among flag, config literals, and env.
func resolveAPIKey(flagKey, modelKey, providerKey, keyEnv string, getenv func(string) string) string {
	for _, k := range []string{flagKey, modelKey, providerKey} {
		if strings.TrimSpace(k) != "" {
			return strings.TrimSpace(k)
		}
	}
	if getenv == nil {
		getenv = os.Getenv
	}
	if keyEnv == "" {
		return ""
	}
	return strings.TrimSpace(getenv(keyEnv))
}

func providerConfigKey(doc *config.Document, provName string) string {
	if doc == nil || provName == "" {
		return ""
	}
	pc, ok := doc.Providers[provName]
	if !ok {
		return ""
	}
	return pc.APIKey
}

// ensureAPIKey resolves or prompts for a key, then Setenv for the process.
func ensureAPIKey(doc *config.Document, agent *config.Agent, provName, flagKey string) (key, keyEnv string, err error) {
	keyEnv = resolveAPIKeyEnv(doc, provName, agent)
	modelKey := ""
	if agent != nil {
		modelKey = agent.Model.APIKey
	}
	key = resolveAPIKey(flagKey, modelKey, providerConfigKey(doc, provName), keyEnv, os.Getenv)
	if key == "" {
		key, err = promptAPIKey(keyEnv)
		if err != nil {
			return "", keyEnv, err
		}
	}
	if key == "" {
		return "", keyEnv, fmt.Errorf("empty API key")
	}
	if err := os.Setenv(keyEnv, key); err != nil {
		return "", keyEnv, err
	}
	return key, keyEnv, nil
}

func promptAPIKey(keyEnv string) (string, error) {
	fmt.Fprintf(os.Stderr, "Enter API key for %s: ", keyEnv)
	fd := int(syscall.Stdin)
	if term.IsTerminal(fd) {
		b, err := term.ReadPassword(fd)
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(b)), nil
	}
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}
