package process

import (
	"encoding/base64"
	"errors"
	"net/url"
	"sort"
)

const (
	maxSecrets           = 64
	maxSecretBytes       = 4096
	maxRegisteredBytes   = 256 * 1024
	redactionReplacement = "[REDACTED]"
)

var errInvalidSecrets = errors.New("invalid secret registration")

// Secret is a value whose fixed set of common serialized forms must be removed
// from command output. Values are compiled before the child process starts.
type Secret struct {
	Value string
}

// Registered forms are the exact value, URL path/query escaping, and standard,
// raw-standard, URL, and raw-URL base64 encodings. Duplicate forms are removed.
func compileSecrets(secrets []Secret) (*redactorFactory, error) {
	if len(secrets) > maxSecrets {
		return nil, errInvalidSecrets
	}

	forms := make(map[string]struct{}, len(secrets)*7)
	total := 0
	for _, secret := range secrets {
		value := secret.Value
		if value == "" || len(value) > maxSecretBytes {
			return nil, errInvalidSecrets
		}
		encoded := []string{
			value,
			url.PathEscape(value),
			url.QueryEscape(value),
			base64.StdEncoding.EncodeToString([]byte(value)),
			base64.RawStdEncoding.EncodeToString([]byte(value)),
			base64.URLEncoding.EncodeToString([]byte(value)),
			base64.RawURLEncoding.EncodeToString([]byte(value)),
		}
		for _, form := range encoded {
			if form == "" {
				continue
			}
			if _, exists := forms[form]; exists {
				continue
			}
			total += len(form)
			if total > maxRegisteredBytes {
				return nil, errInvalidSecrets
			}
			forms[form] = struct{}{}
		}
	}

	patterns := make([]string, 0, len(forms))
	for form := range forms {
		patterns = append(patterns, form)
	}
	sort.Strings(patterns)

	root := &redactionNode{}
	for _, pattern := range patterns {
		root.insert([]byte(pattern))
	}
	return &redactorFactory{root: root}, nil
}

type redactorFactory struct {
	root *redactionNode
}

func (f *redactorFactory) new() *streamRedactor {
	return &streamRedactor{root: f.root}
}

type redactionNode struct {
	children map[byte]*redactionNode
	terminal bool
}

func (n *redactionNode) insert(pattern []byte) {
	current := n
	for _, value := range pattern {
		if current.children == nil {
			current.children = make(map[byte]*redactionNode)
		}
		next := current.children[value]
		if next == nil {
			next = &redactionNode{}
			current.children[value] = next
		}
		current = next
	}
	current.terminal = true
}

type streamRedactor struct {
	root    *redactionNode
	pending []byte
}

func (r *streamRedactor) Write(data []byte, emit func([]byte) error) error {
	r.pending = append(r.pending, data...)
	return r.process(false, emit)
}

func (r *streamRedactor) Flush(emit func([]byte) error) error {
	return r.process(true, emit)
}

func (r *streamRedactor) process(flush bool, emit func([]byte) error) error {
	for len(r.pending) > 0 {
		matched, consumed, needMore := r.match(flush)
		if needMore {
			return nil
		}
		if matched {
			if err := emit([]byte(redactionReplacement)); err != nil {
				return err
			}
			r.pending = r.pending[consumed:]
			continue
		}
		if err := emit(r.pending[:1]); err != nil {
			return err
		}
		r.pending = r.pending[1:]
	}
	return nil
}

func (r *streamRedactor) match(flush bool) (matched bool, consumed int, needMore bool) {
	node := r.root
	longest := 0
	for index, value := range r.pending {
		next := node.children[value]
		if next == nil {
			break
		}
		node = next
		if node.terminal {
			longest = index + 1
		}
		if index == len(r.pending)-1 && !flush && len(node.children) > 0 {
			return false, 0, true
		}
	}
	if longest > 0 {
		return true, longest, false
	}
	if !flush && len(r.pending) > 0 {
		node = r.root
		for index, value := range r.pending {
			next := node.children[value]
			if next == nil {
				break
			}
			node = next
			if index == len(r.pending)-1 && len(node.children) > 0 {
				return false, 0, true
			}
		}
	}
	return false, 0, false
}
