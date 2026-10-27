package credentials

import "context"

// Provider stores and retrieves SCM credentials for an OpenShift caller.
type Provider interface {
	SaveCredential(ctx context.Context, ocpToken string, cred Credential) error
	GetCredential(ctx context.Context, ocpToken string) (Credential, error)
	DeleteCredential(ctx context.Context, ocpToken string) error
}
