// Package config reads sd-report-collector's configuration following the UAPI.6
// Configuration Files Specification, via the libeconf C library.
package config

/*
#cgo pkg-config: libeconf
#include <libeconf.h>
#include <stdlib.h>
*/
import "C"

import (
	"errors"
	"fmt"
	"sort"
	"unsafe"
)

// ErrNoConfigFile is returned by readConfig when no configuration file was
// found anywhere in the UAPI.6 search path.
var ErrNoConfigFile = errors.New("no configuration file found")

// keyFile wraps a parsed econf_file, merged across the UAPI.6 hierarchy.
type keyFile struct {
	ptr *C.econf_file
}

func econfError(rc C.econf_err) error {
	if rc == C.ECONF_SUCCESS {
		return nil
	}
	return fmt.Errorf("libeconf: %s", C.GoString(C.econf_errString(rc)))
}

// readConfig loads and merges "<project>/<configName>.<configSuffix>" from
// /usr/lib (usrSubdir), /run and /etc, including ".d" drop-ins, per UAPI.6.
func readConfig(project, usrSubdir, configName, configSuffix string) (*keyFile, error) {
	cProject := C.CString(project)
	defer C.free(unsafe.Pointer(cProject))
	cUsrSubdir := C.CString(usrSubdir)
	defer C.free(unsafe.Pointer(cUsrSubdir))
	cConfigName := C.CString(configName)
	defer C.free(unsafe.Pointer(cConfigName))
	cConfigSuffix := C.CString(configSuffix)
	defer C.free(unsafe.Pointer(cConfigSuffix))
	cDelim := C.CString("=")
	defer C.free(unsafe.Pointer(cDelim))
	cComment := C.CString("#")
	defer C.free(unsafe.Pointer(cComment))

	var kf *C.econf_file
	rc := C.econf_readConfig(&kf, cProject, cUsrSubdir, cConfigName, cConfigSuffix, cDelim, cComment)
	if rc == C.ECONF_NOFILE {
		return nil, ErrNoConfigFile
	}
	if rc != C.ECONF_SUCCESS {
		return nil, econfError(rc)
	}
	return &keyFile{ptr: kf}, nil
}

// readFile parses a single config file directly (bypassing the UAPI.6
// hierarchy search), for use in tests where rooting real /usr, /run, /etc
// trees isn't practical.
func readFile(path string) (*keyFile, error) {
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))
	cDelim := C.CString("=")
	defer C.free(unsafe.Pointer(cDelim))
	cComment := C.CString("#")
	defer C.free(unsafe.Pointer(cComment))

	var kf *C.econf_file
	rc := C.econf_readFile(&kf, cPath, cDelim, cComment)
	if rc != C.ECONF_SUCCESS {
		return nil, econfError(rc)
	}
	return &keyFile{ptr: kf}, nil
}

func (k *keyFile) Close() {
	if k.ptr != nil {
		C.econf_freeFile(k.ptr)
		k.ptr = nil
	}
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
	cGroup := C.CString(group)
	defer C.free(unsafe.Pointer(cGroup))
	cKey := C.CString(key)
	defer C.free(unsafe.Pointer(cKey))
	cDef := C.CString(def)
	defer C.free(unsafe.Pointer(cDef))

	var result *C.char
	rc := C.econf_getStringValueDef(k.ptr, cGroup, cKey, &result, cDef)
	if rc != C.ECONF_SUCCESS && rc != C.ECONF_NOKEY {
		return "", false, econfError(rc)
	}
	defer C.free(unsafe.Pointer(result))
	return C.GoString(result), rc == C.ECONF_SUCCESS, nil
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
	cGroup := C.CString(group)
	defer C.free(unsafe.Pointer(cGroup))
	cKey := C.CString(key)
	defer C.free(unsafe.Pointer(cKey))

	var result C.int64_t
	rc := C.econf_getInt64ValueDef(k.ptr, cGroup, cKey, &result, C.int64_t(def))
	if rc != C.ECONF_SUCCESS && rc != C.ECONF_NOKEY {
		return 0, econfError(rc)
	}
	return int64(result), nil
}

// getKeys returns the sorted key names of the given group. An absent group
// yields an empty slice, not an error.
func (k *keyFile) getKeys(group string) ([]string, error) {
	cGroup := C.CString(group)
	defer C.free(unsafe.Pointer(cGroup))

	var length C.size_t
	var keys **C.char
	rc := C.econf_getKeys(k.ptr, cGroup, &length, &keys)
	if rc == C.ECONF_NOGROUP || rc == C.ECONF_NOKEY {
		return nil, nil
	}
	if rc != C.ECONF_SUCCESS {
		return nil, econfError(rc)
	}
	defer C.econf_freeArray(keys)

	result := make([]string, 0, int(length))
	for _, cstr := range unsafe.Slice(keys, int(length)) {
		result = append(result, C.GoString(cstr))
	}
	sort.Strings(result)
	return result, nil
}
