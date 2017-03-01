package pki

type TLScertType uint

const (
	TLS_SRVONLY TLScertType = iota
	TLS_CLIONLY
	TLS_CLISRV
)
