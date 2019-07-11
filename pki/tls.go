package pki

// updated, delete me

type TLScertType uint

const (
	TLS_SRVONLY TLScertType = iota
	TLS_CLIONLY
	TLS_CLISRV
)
