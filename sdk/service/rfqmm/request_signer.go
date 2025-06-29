package rfqmm

import (
	"math/big"

	ethutils "github.com/celer-network/goutils/eth"
	"github.com/celer-network/intent-rfq-mm/sdk/eth"
	"github.com/celer-network/intent-rfq-mm/sdk/service/rfqmm/proto"
)

type RequestSignerConfig struct {
	ChainId    uint64
	Keystore   string
	Passphrase string
}

// DefaultRequestSigner is a default implementation of interface RequestSigner.
type DefaultRequestSigner struct {
	Signer  ethutils.Signer
	Address eth.Addr
}

// NewRequestSigner creates a new instance of DefaultRequestSigner.
func NewRequestSigner(config *RequestSignerConfig) *DefaultRequestSigner {
	signer, addr, err := createSigner(config.Keystore, config.Passphrase, big.NewInt(int64(config.ChainId)))
	if err != nil {
		panic(err)
	}
	return &DefaultRequestSigner{
		Signer:  signer,
		Address: addr,
	}
}

var _ RequestSigner = &DefaultRequestSigner{}

// Sign Method returns the signature of the underlying signer for the given data.
func (rs *DefaultRequestSigner) Sign(data []byte) ([]byte, error) {
	sig, err := rs.Signer.SignEthMessage(data)
	if err != nil {
		return nil, proto.NewErr(proto.ErrCode_ERROR_REQUEST_SIGNER, err.Error())
	}
	return sig, nil
}

// Verify Method returns whether the signature is signed by the underlying signer.
func (rs *DefaultRequestSigner) Verify(data, sig []byte) bool {
	addr, err := ethutils.RecoverSigner(data, sig)
	if err != nil {
		return false
	}
	if rs.Address != addr {
		return false
	}
	return true
}
