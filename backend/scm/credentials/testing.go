package credentials

import (
	"context"
	"errors"

	"github.com/openshift/faas-console-plugin/backend/identity"
)

// ProviderStub supplies credential operations for tests that exercise failures.
type ProviderStub struct {
	OnSaveCredential   func(context.Context, string, Credential) error
	OnGetCredential    func(context.Context, string) (Credential, error)
	OnDeleteCredential func(context.Context, string) error
}

func (p ProviderStub) SaveCredential(ctx context.Context, ocpToken string, cred Credential) error {
	if p.OnSaveCredential != nil {
		return p.OnSaveCredential(ctx, ocpToken, cred)
	}
	return errors.New("save credential not implemented")
}

func (p ProviderStub) GetCredential(ctx context.Context, ocpToken string) (Credential, error) {
	if p.OnGetCredential != nil {
		return p.OnGetCredential(ctx, ocpToken)
	}
	return Credential{}, errors.New("get credential not implemented")
}

func (p ProviderStub) DeleteCredential(ctx context.Context, ocpToken string) error {
	if p.OnDeleteCredential != nil {
		return p.OnDeleteCredential(ctx, ocpToken)
	}
	return errors.New("delete credential not implemented")
}

// MapProvider is a test fake keyed by caller token; identity binding is tested
// separately against Store, which resolves tokens to OpenShift users.
type MapProvider map[string]Credential

func (m MapProvider) SaveCredential(_ context.Context, ocpToken string, cred Credential) error {
	if ocpToken == "" {
		return identity.ErrUnauthenticated
	}
	m[ocpToken] = cred
	return nil
}

func (m MapProvider) GetCredential(_ context.Context, ocpToken string) (Credential, error) {
	if ocpToken == "" {
		return Credential{}, identity.ErrUnauthenticated
	}
	if cred, ok := m[ocpToken]; ok {
		return cred, nil
	}
	return Credential{}, ErrNoCredential
}

func (m MapProvider) DeleteCredential(_ context.Context, ocpToken string) error {
	if ocpToken == "" {
		return identity.ErrUnauthenticated
	}
	delete(m, ocpToken)
	return nil
}
