package rpc

import (
	"fmt"

	credentialapp "github.com/tinoosan/agen8/internal/services/credential/app"
	credentialrpc "github.com/tinoosan/agen8/internal/services/credential/rpc"
)

const (
	MethodCredentialList   = "credential.list"   // #nosec G101 -- RPC method name, not a credential.
	MethodCredentialGet    = "credential.get"    // #nosec G101 -- RPC method name, not a credential.
	MethodCredentialCreate = "credential.create" // #nosec G101 -- RPC method name, not a credential.
	MethodCredentialUpdate = "credential.update" // #nosec G101 -- RPC method name, not a credential.
	MethodCredentialDelete = "credential.delete" // #nosec G101 -- RPC method name, not a credential.
)

func RegisterCredential(reg *Registry, credentialSvc *credentialapp.Service) error {
	if credentialSvc == nil {
		return fmt.Errorf("credential service is required")
	}
	handler := credentialrpc.MustNewHandler(credentialSvc)
	return RegisterHandlers(
		func() error {
			return AddBoundHandler(reg, MethodCredentialList, true, handler.CredentialList)
		},
		func() error {
			return AddBoundHandler(reg, MethodCredentialGet, false, handler.CredentialGet)
		},
		func() error {
			return AddBoundHandler(reg, MethodCredentialCreate, false, handler.CredentialCreate)
		},
		func() error {
			return AddBoundHandler(reg, MethodCredentialUpdate, false, handler.CredentialUpdate)
		},
		func() error {
			return AddBoundHandler(reg, MethodCredentialDelete, false, handler.CredentialDelete)
		},
	)
}
