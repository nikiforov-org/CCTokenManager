package main

// Secrets on Windows are generic credentials named service/name, kept as Bun,
// which the CLI runs on, keeps them: one too long for a credential goes in base64
// parts, #0, #1 and on, with #m giving their count and length.

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

// storeName is what the system calls the place the tokens are kept.
const storeName = "Credential Manager"

var (
	pCredReadW      = advapi32.NewProc("CredReadW")
	pCredWriteW     = advapi32.NewProc("CredWriteW")
	pCredDeleteW    = advapi32.NewProc("CredDeleteW")
	pCredEnumerateW = advapi32.NewProc("CredEnumerateW")
	pCredFree       = advapi32.NewProc("CredFree")
)

// credential is CREDENTIALW.
type credential struct {
	Flags, Type        uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        [2]uint32
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

const (
	credTypeGeneric         = 1
	credPersistLocalMachine = 2
	errorNotFound           = syscall.Errno(1168)
	// partSize is the most of a value the CLI puts in one credential, which
	// holds 2560 bytes, and maxParts the most parts it reads.
	partSize = 2400
	maxParts = 256
)

// loadToken is the profile's token, or "" if Credential Manager has none for
// it.
func loadToken(id string) (string, error) { return loadSecret("CC Token Manager", id) }

// storeToken puts the profile's token in Credential Manager, and an empty one
// takes it out.
func storeToken(id, token string) error { return storeSecret("CC Token Manager", id, token) }

// loadSecret is the value kept for service and name, put together if it is in
// parts, or "" if there is none.
func loadSecret(service, name string) (string, error) {
	target := service + "/" + name
	head, err := credRead(target + "#m")
	if err != nil {
		return "", err
	}
	var parts struct{ N, L int }
	if json.Unmarshal([]byte(head), &parts) != nil || parts.N < 1 || parts.N > maxParts {
		return credRead(target)
	}
	var b strings.Builder
	for i := range parts.N {
		part, err := credRead(target + "#" + strconv.Itoa(i))
		if err != nil {
			return "", err
		}
		b.WriteString(part)
	}
	value, err := base64.StdEncoding.DecodeString(b.String())
	if err != nil || b.Len() != parts.L {
		return "", fmt.Errorf("the parts of %s do not add up", target)
	}
	return string(value), nil
}

// storeSecret keeps value for service and name, in parts if it is too long for
// one credential, and an empty one takes it out. Whatever else was kept for
// them goes.
func storeSecret(service, name, value string) error {
	target := service + "/" + name
	switch {
	case value == "":
		dropParts(target)
		return credDelete(target)
	case len(value) <= partSize:
		// Written before the parts of a longer value go, which would be read
		// first: a reader meanwhile finds one or the other.
		if err := credWrite(target, name, value); err != nil {
			return err
		}
		dropParts(target)
		return nil
	}
	b := base64.StdEncoding.EncodeToString([]byte(value))
	n := (len(b) + partSize - 1) / partSize
	dropParts(target)
	for i := range n {
		suffix := "#" + strconv.Itoa(i)
		if err := credWrite(target+suffix, name+suffix, b[i*partSize:min(len(b), (i+1)*partSize)]); err != nil {
			return err
		}
	}
	head, _ := json.Marshal(map[string]int{"n": n, "l": len(b)})
	if err := credWrite(target+"#m", name+"#m", string(head)); err != nil {
		return err
	}
	return credDelete(target)
}

// dropParts takes out every part kept for target.
func dropParts(target string) {
	var n uint32
	var list **credential
	if r, _, _ := pCredEnumerateW.Call(uintptr(unsafe.Pointer(u16(target+"#*"))), 0,
		uintptr(unsafe.Pointer(&n)), uintptr(unsafe.Pointer(&list))); r == 0 {
		return
	}
	defer pCredFree.Call(uintptr(unsafe.Pointer(list)))
	for _, c := range unsafe.Slice(list, n) {
		pCredDeleteW.Call(uintptr(unsafe.Pointer(c.TargetName)), credTypeGeneric, 0)
	}
}

// credRead is what the credential target holds, or "" if there is none.
func credRead(target string) (string, error) {
	var c *credential
	if r, _, err := pCredReadW.Call(uintptr(unsafe.Pointer(u16(target))), credTypeGeneric, 0, uintptr(unsafe.Pointer(&c))); r == 0 {
		if err == errorNotFound {
			return "", nil
		}
		return "", err
	}
	defer pCredFree.Call(uintptr(unsafe.Pointer(c)))
	return string(unsafe.Slice(c.CredentialBlob, c.CredentialBlobSize)), nil
}

// credWrite puts value, which is not empty, in the credential target, filed
// under name.
func credWrite(target, name, value string) error {
	blob := []byte(value)
	c := credential{
		Type:               credTypeGeneric,
		TargetName:         u16(target),
		CredentialBlobSize: uint32(len(blob)),
		CredentialBlob:     &blob[0],
		Persist:            credPersistLocalMachine,
		UserName:           u16(name),
	}
	if r, _, err := pCredWriteW.Call(uintptr(unsafe.Pointer(&c)), 0); r == 0 {
		return err
	}
	return nil
}

// credDelete takes out the credential target, if there is one.
func credDelete(target string) error {
	if r, _, err := pCredDeleteW.Call(uintptr(unsafe.Pointer(u16(target))), credTypeGeneric, 0); r == 0 && err != errorNotFound {
		return err
	}
	return nil
}
