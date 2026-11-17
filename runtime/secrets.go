package runtime

import (
	"time"

	"github.com/kasra-sh/zendawn-go/config"
	"github.com/kasra-sh/zendawn-go/secret"
	"github.com/kasra-sh/zendawn-go/zerr"
)

// SecretAskResolver collects a secret value from the host (masked UI).
type SecretAskResolver func(name, description, reason string) (string, error)

// InitSecrets builds the vault from document declarations and wires secret.ask.
func (r *Runtime) InitSecrets() {
	if r.Secrets != nil {
		return
	}
	var decls []config.SecretDecl
	if r.Doc != nil {
		decls = r.Doc.Secrets
	}
	r.Secrets = secret.New(decls)
	r.wireSecretAsk()
}

// ProvideSecret stores a value and unblocks a waiting secret.ask channel.
func (r *Runtime) ProvideSecret(name, value string) {
	r.InitSecrets()
	r.Secrets.Provide(name, value)
	r.mu.Lock()
	ch := r.secretWait
	r.secretWait = nil
	r.mu.Unlock()
	if ch != nil {
		select {
		case ch <- value:
		default:
		}
	}
}

func (r *Runtime) wireSecretAsk() {
	if r.Secrets == nil {
		return
	}
	r.Secrets.Ask = func(name, description, reason string) (string, error) {
		sessionID := r.secretSessionID
		r.emit(NewEvent(EventSecretAsk, sessionID, map[string]any{
			"name":        name,
			"description": description,
			"reason":      reason,
		}))
		if r.SecretAsk != nil {
			v, err := r.SecretAsk(name, description, reason)
			ok := err == nil && v != ""
			data := map[string]any{"name": name, "ok": ok}
			r.emit(NewEvent(EventSecretResolved, sessionID, data))
			if err != nil {
				return "", err
			}
			if v == "" {
				return "", zerr.New(zerr.CodeMissingSecret, name, false)
			}
			return v, nil
		}
		ch := make(chan string, 1)
		r.mu.Lock()
		r.secretWait = ch
		r.mu.Unlock()

		timeoutMs := 0
		if r.Doc != nil && r.Doc.Runtime != nil && r.Doc.Runtime.AskTimeoutMs != nil {
			timeoutMs = *r.Doc.Runtime.AskTimeoutMs
		}
		if timeoutMs > 0 {
			select {
			case v := <-ch:
				r.emit(NewEvent(EventSecretResolved, sessionID, map[string]any{"name": name, "ok": true}))
				return v, nil
			case <-time.After(time.Duration(timeoutMs) * time.Millisecond):
				r.emit(NewEvent(EventSecretResolved, sessionID, map[string]any{
					"name": name, "ok": false, "timedOut": true,
				}))
				return "", zerr.New(zerr.CodeMissingSecret, name, false)
			}
		}
		v := <-ch
		r.emit(NewEvent(EventSecretResolved, sessionID, map[string]any{"name": name, "ok": true}))
		return v, nil
	}
}
