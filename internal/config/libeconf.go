// Package config reads sd-report-collector's configuration following the UAPI.6
// Configuration Files Specification, via libeconf's native Go package.
package config

import (
	"errors"
	"fmt"
	"sort"

	"github.com/openSUSE/libeconf/go/econf"
)

// ErrNoConfigFile is returned by readConfig when no configuration file was
// found anywhere in the UAPI.6 search path.
var ErrNoConfigFile = errors.New("no configuration file found")

// keyFile wraps a parsed econf.File, merged across the UAPI.6 hierarchy.
type keyFile struct {
	f *econf.File
}

// readConfig loads and merges "<project>/<configName>.<configSuffix>" from
// /usr/lib (usrSubdir), /run and /etc, including ".d" drop-ins, per UAPI.6.
func readConfig(project, usrSubdir, configName, configSuffix string) (*keyFile, error) {
	f, err := econf.ReadConfig(project, usrSubdir, configName, configSuffix, "=", "#")
	if errors.Is(err, econf.ErrNoFile) {
		return nil, ErrNoConfigFile
	}
	if err != nil {
		return nil, fmt.Errorf("libeconf: %w", err)
	}
	return &keyFile{f: f}, nil
}

// readFile parses a single config file directly (bypassing the UAPI.6
// hierarchy search), for use in tests where rooting real /usr, /run, /etc
// trees isn't practical.
func readFile(path string) (*keyFile, error) {
	f, err := econf.ReadFile(path, "=", "#")
	if err != nil {
		return nil, fmt.Errorf("libeconf: %w", err)
	}
	return &keyFile{f: f}, nil
}

func (k *keyFile) Close() {
	_ = k.f.Close()
}

// getString returns the string value for group/key, or def if absent.
func (k *keyFile) getString(group, key, def string) (string, error) {
	val, _, err := k.getStringIn(group, key, def)
	return val, err
}

// getStringIn returns the string value for group/key, and whether the key
// was actually present in that group (as opposed to def being returned
// because it was absent).
func (k *keyFile) getStringIn(group, key, def string) (string, bool, error) {
	val, err := k.f.StringDef(group, key, def)
	if errors.Is(err, econf.ErrNoKey) {
		return val, false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("libeconf: %w", err)
	}
	return val, true, nil
}

// getStringFallback looks up key in each of groups in order, returning the
// value from the first group that has it set, or def if none do.
func (k *keyFile) getStringFallback(groups []string, key, def string) (string, error) {
	for _, group := range groups {
		val, found, err := k.getStringIn(group, key, def)
		if err != nil {
			return "", err
		}
		if found {
			return val, nil
		}
	}
	return def, nil
}

// getInt returns the int value for group/key, or def if absent.
func (k *keyFile) getInt(group, key string, def int64) (int64, error) {
	val, err := k.f.Int64Def(group, key, def)
	if errors.Is(err, econf.ErrNoKey) {
		return val, nil
	}
	if err != nil {
		return 0, fmt.Errorf("libeconf: %w", err)
	}
	return val, nil
}

// getBool returns the bool value for group/key, or def if absent.
func (k *keyFile) getBool(group, key string, def bool) (bool, error) {
	val, err := k.f.BoolDef(group, key, def)
	if errors.Is(err, econf.ErrNoKey) {
		return val, nil
	}
	if err != nil {
		return false, fmt.Errorf("libeconf: %w", err)
	}
	return val, nil
}

// getKeys returns the sorted key names of the given group. An absent group
// yields an empty slice, not an error.
func (k *keyFile) getKeys(group string) ([]string, error) {
	keys, err := k.f.Keys(group)
	if errors.Is(err, econf.ErrNoGroup) || errors.Is(err, econf.ErrNoKey) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("libeconf: %w", err)
	}
	sort.Strings(keys)
	return keys, nil
}
