package commands

import "context"

type contextKey string

const globalFlagsKey contextKey = "globalFlags"

// GlobalFlags holds global CLI flags.
type GlobalFlags struct {
	ConfigFile         string
	ClientID           string
	ClientSecret       string
	APIURL             string
	TargetClientID     string
	TargetClientSecret string
	TargetAPIURL       string
	Debug              bool
	NoColor            bool
	Quiet              bool
	Verbose            bool
	Yes                bool
	NoEnvFile          bool
}

// WithGlobalFlags adds global flags to the context.
func WithGlobalFlags(ctx context.Context, flags GlobalFlags) context.Context {
	return context.WithValue(ctx, globalFlagsKey, flags)
}

// GetGlobalFlags retrieves global flags from context.
func GetGlobalFlags(ctx context.Context) GlobalFlags {
	flags, ok := ctx.Value(globalFlagsKey).(GlobalFlags)
	if !ok {
		return GlobalFlags{}
	}
	return flags
}
