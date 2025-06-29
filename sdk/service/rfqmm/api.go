package rfqmm

import (
	"context"
	"math/big"
	"time"

	"github.com/celer-network/goutils/log"
	"github.com/celer-network/intent-rfq-mm/sdk/eth"
	"github.com/celer-network/intent-rfq-mm/sdk/service/rfqmm/proto"
	solsha3 "github.com/miguelmota/go-solidity-sha3"
	"google.golang.org/grpc"
)

const BestPeriodMultiplier = 1.2

// Price API is used to get price from MM for a swap within PriceRequest.
// In PriceRequest, at least one amount should be given.
//   - If SrcAmount is given, MM application will return with how much DstToken will be received by User.
//   - If DstAmount is given, MM application will return with how much SrcToken User should deposit to receive
//     such amount of DstToken. (not yet implemented)
//   - If both of SrcAmount and DstAmount is given, MM application will treat it as the first case.
func (c *Client) Price(ctx context.Context, in *proto.PriceRequest, opts ...grpc.CallOption) (*proto.PriceResponse, error) {
	if ok, reason := validatePriceRequest(in); !ok {
		return &proto.PriceResponse{Err: proto.NewErr(proto.ErrCode_ERROR_INVALID_ARGUMENTS, reason).ToCommonErr()}, nil
	}
	return c.ApiClient.Price(ctx, in, opts...)
}

// Quote API is used to confirm a quotation from MM.
func (c *Client) Quote(ctx context.Context, in *proto.QuoteRequest, opts ...grpc.CallOption) (*proto.QuoteResponse, error) {
	if ok, reason := validateQuoteRequest(in); !ok {
		return &proto.QuoteResponse{Err: proto.NewErr(proto.ErrCode_ERROR_INVALID_ARGUMENTS, reason).ToCommonErr()}, nil
	}
	return c.ApiClient.Quote(ctx, in, opts...)
}

// Price API is a default implementation of responding a Client.Price request.
//
// Basic flow:
//   - validate price request
//   - calculate price
//   - check if there is sufficient liquidity for requested token
//   - sign price
func (s *Server) Price(ctx context.Context, request *proto.PriceRequest) (response *proto.PriceResponse, err error) {
	defer func() {
		if response.Err == nil {
			log.Debugf("Price with success, price %s", response.Price.String())
		} else {
			log.Debugf("Price with failure, err:%s, request %s", response.Err.String(), request.String())
		}
	}()
	if ok, reason := validatePriceRequest(request); !ok {
		return &proto.PriceResponse{Err: proto.NewErr(proto.ErrCode_ERROR_INVALID_ARGUMENTS, reason).ToCommonErr()}, nil
	}
	if !s.LiquidityProvider.HasTokenPair(request.SrcToken, request.DstToken) {
		return &proto.PriceResponse{Err: proto.NewErr(proto.ErrCode_ERROR_INVALID_ARGUMENTS, "unsupported token pair").ToCommonErr()}, nil
	}
	sendAmount := new(big.Int)
	releaseAmount := new(big.Int)
	receiveAmount := new(big.Int)
	baseFee := new(big.Int)
	fee := new(big.Int)
	// switch mod, one is sendAmt => receiveAmt, the other one is receiveAmt => sendAmt
	if request.SrcAmount == "" {
		// todo, not supported now
		receiveAmount.SetString(request.DstAmount, 10)
		baseFee.SetString(request.BaseFee, 10)
		sendAmount, releaseAmount, fee, err = s.AmountCalculator.CalSendAmt(request.SrcToken, request.DstToken, receiveAmount, baseFee, s.Config.LightMM)
		if err != nil {
			return &proto.PriceResponse{Err: err.(*proto.Err).ToCommonErr()}, nil
		}
	} else {
		sendAmount.SetString(request.SrcAmount, 10)
		baseFee.SetString(request.BaseFee, 10)
		receiveAmount, releaseAmount, fee, err = s.AmountCalculator.CalRecvAmt(request.SrcToken, request.DstToken, sendAmount, baseFee, s.Config.LightMM)
		if err != nil {
			return &proto.PriceResponse{Err: err.(*proto.Err).ToCommonErr()}, nil
		}
	}
	mmAddr, err := s.LiquidityProvider.GetLiquidityProviderAddr(request.SrcToken.ChainId)
	if err != nil {
		return &proto.PriceResponse{Err: err.(*proto.Err).ToCommonErr()}, nil
	}
	dstTokenAddr := request.DstToken.GetAddr()
	freezeTime, err := s.LiquidityProvider.AskForFreezing(request.DstToken.ChainId, dstTokenAddr, receiveAmount, request.DstNative)
	if err != nil {
		return &proto.PriceResponse{Err: err.(*proto.Err).ToCommonErr()}, nil
	}

	price := &proto.Price{
		SrcToken:          request.SrcToken,
		SrcAmount:         sendAmount.String(),
		SrcReleaseAmount:  releaseAmount.String(),
		DstToken:          request.DstToken,
		DstAmount:         receiveAmount.String(),
		FeeAmount:         fee.String(),
		ValidThru:         time.Now().Unix() + s.Config.PriceValidPeriod,
		MmAddr:            mmAddr.String(),
		Sig:               "",
		SrcDepositPeriod:  int64(float64(freezeTime) / BestPeriodMultiplier),
		DstTransferPeriod: int64(BestPeriodMultiplier * float64(s.Config.DstTransferPeriod)),
	}
	sigBytes, err := s.RequestSigner.Sign(price.EncodeSignData())
	if err != nil {
		return &proto.PriceResponse{Err: err.(*proto.Err).ToCommonErr()}, nil
	}
	price.Sig = eth.Bytes2Hex(sigBytes)
	return &proto.PriceResponse{Price: price}, nil
}

// Quote service is a default implementation of responding at a Client.Quote request.
//
// Basic flow:
//   - validate quote request
//   - check price sig
//   - check release amount within request is correct
//   - check if there is sufficient liquidity for requested token
//   - sign quote
func (s *Server) Quote(ctx context.Context, request *proto.QuoteRequest) (response *proto.QuoteResponse, err error) {
	defer func() {
		if response.Err == nil {
			log.Infof("Quote with success, quote %s", request.Quote.String())
		} else {
			log.Warnf("Quote with failure, err:%s, quote %s", response.Err.String(), request.Quote.String())
		}
	}()
	if ok, reason := validateQuoteRequest(request); !ok {
		return &proto.QuoteResponse{Err: proto.NewErr(proto.ErrCode_ERROR_INVALID_ARGUMENTS, reason).ToCommonErr()}, nil
	}
	price := request.Price
	quote := request.Quote
	if !s.RequestSigner.Verify(price.EncodeSignData(), eth.Hex2Bytes(price.Sig)) {
		return &proto.QuoteResponse{Err: proto.NewErr(proto.ErrCode_ERROR_INVALID_ARGUMENTS, "invalid sig").ToCommonErr()}, nil
	}
	if !quote.ValidateQuoteHash() {
		return &proto.QuoteResponse{Err: proto.NewErr(proto.ErrCode_ERROR_INVALID_ARGUMENTS, "invalid quote hash").ToCommonErr()}, nil
	}
	dstAmt := price.GetDstAmt()
	dstTokenAddr := request.Price.DstToken.GetAddr()
	freezeTime, err := s.LiquidityProvider.AskForFreezing(price.GetDstChainId(), dstTokenAddr, dstAmt, request.DstNative)
	if err != nil {
		return &proto.QuoteResponse{Err: err.(*proto.Err).ToCommonErr()}, nil
	}
	if time.Now().Unix()+freezeTime < quote.SrcDeadline {
		return &proto.QuoteResponse{Err: proto.NewErr(proto.ErrCode_ERROR_INVALID_ARGUMENTS, "srcDeadline too large").ToCommonErr()}, nil
	}
	if time.Now().Unix()+s.Config.DstTransferPeriod > quote.DstDeadline {
		return &proto.QuoteResponse{Err: proto.NewErr(proto.ErrCode_ERROR_INVALID_ARGUMENTS, "dstDeadline too small").ToCommonErr()}, nil
	}
	sigBytes, err := s.RequestSigner.Sign(quote.GetQuoteHash().Bytes())
	if err != nil {
		return &proto.QuoteResponse{Err: err.(*proto.Err).ToCommonErr()}, nil
	}
	// no freeze before user deposit token
	//err = s.LiquidityProvider.FreezeLiquidity(price.GetDstChainId(), dstTokenAddr, dstAmt, quote.SrcDeadline, quote.GetQuoteHash(), request.DstNative)
	//if err != nil {
	//	return &proto.QuoteResponse{Err: err.(*proto.Err).ToCommonErr()}, nil
	//}
	return &proto.QuoteResponse{QuoteSig: eth.Bytes2Hex(sigBytes)}, nil
}

// SignQuoteHash service is a default implementation of responding at a Client.SignQuoteHash request.
//
// Basic flow:
//   - check if self is a light versioned market maker
//   - check quote sig
//   - check dst deadline of quote
//   - check deposit tx of user is mined on src chain and expected event is emitted
//   - check quote status within rfq contract on src chain is 1(SrcDeposited)
//   - sign quote
func (s *Server) SignQuoteHash(ctx context.Context, request *proto.SignQuoteHashRequest) (*proto.SignQuoteHashResponse, error) {
	if !s.Config.LightMM {
		return signQuoteHashArgumentErr("this api only works for light mm")
	}
	dstChainId := request.GetQuote().GetDstChainId()
	rfqContract, err := s.ChainCaller.GetRfqContract(dstChainId)
	if err != nil {
		return signQuoteHashArgumentErr(err.Error())
	}

	// check quote sig
	quote := request.Quote
	err = s.checkQuoteSig(quote, request.QuoteSig)
	if err != nil {
		return signQuoteHashArgumentErr(err.Error())
	}

	// check quote
	err = s.checkQuote(quote, request.GetSrcDepositTxHash(), false)
	if err != nil {
		return signQuoteHashArgumentErr(err.Error())
	}

	data := EncodeDataToSign(dstChainId, rfqContract, quote.GetQuoteHash())
	sig, err := s.RequestSigner.Sign(data)
	if err != nil {
		return &proto.SignQuoteHashResponse{
			Err: err.(*proto.Err).ToCommonErr(),
		}, nil
	}
	if sig[64] <= 1 {
		// Use 27/28 for v to be compatible with openzeppelin ECDSA lib
		sig[64] = sig[64] + 27
	}
	return &proto.SignQuoteHashResponse{
		Sig: sig,
	}, nil
}

func signQuoteHashArgumentErr(reason string) (*proto.SignQuoteHashResponse, error) {
	return &proto.SignQuoteHashResponse{Err: proto.NewErr(proto.ErrCode_ERROR_INVALID_ARGUMENTS, reason).ToCommonErr()}, nil
}

// Tokens service is a default implementation of responding at a Client.Tokens request.
//
// Basic flow:
//   - return all supported tokens
func (s *Server) Tokens(ctx context.Context, request *proto.TokensRequest) (*proto.TokensResponse, error) {
	return &proto.TokensResponse{
		Tokens: s.LiquidityProvider.GetTokens(),
	}, nil
}

func EncodeDataToSign(dstChainId uint64, dstAddr eth.Addr, data eth.Hash) []byte {
	return solsha3.Pack(
		[]string{"uint256", "address", "string", "bytes32"},
		[]interface{}{new(big.Int).SetUint64(dstChainId), dstAddr, "AllowedTransfer", data},
	)
}

func validatePriceRequest(request *proto.PriceRequest) (bool, string) {
	if request.SrcToken == nil || request.DstToken == nil {
		return false, "SrcToken or DstToken is nil"
	}
	if request.SrcAmount == "" && request.DstAmount == "" {
		return false, "SrcAmount and DstAmount are both empty"
	}
	if request.SrcAmount == "" {
		if _, ok := new(big.Int).SetString(request.DstAmount, 10); !ok {
			return false, "invalid SrcAmount"
		}
	} else {
		if _, ok := new(big.Int).SetString(request.SrcAmount, 10); !ok {
			return false, "invalid DstAmount"
		}
	}
	return true, ""
}

func validateQuoteRequest(request *proto.QuoteRequest) (bool, string) {
	price := request.Price
	quote := request.Quote
	if request.Price == nil || request.Quote == nil {
		return false, "price or quote is nil"
	}
	if price.SrcToken == nil || price.DstToken == nil {
		return false, "price.SrcToken or price.DstToken is nil"
	}
	if price.SrcAmount == "" || price.DstAmount == "" || price.SrcReleaseAmount == "" {
		return false, "price.SrcAmount, price.DstAmount or price.SrcReleaseAmount is empty"
	}
	if time.Now().Unix() > price.ValidThru {
		return false, "past price valid time"
	}
	if price.GetMMAddr() != quote.GetMMAddr() {
		return false, "mm addr mismatch"
	}
	if !quote.SrcToken.EqualBasically(price.SrcToken) || !quote.DstToken.EqualBasically(price.DstToken) {
		return false, "token in price and quote mismatch"
	}
	if quote.SrcAmount != price.SrcAmount || quote.DstAmount != price.DstAmount || quote.SrcReleaseAmount != price.SrcReleaseAmount {
		return false, "amount in price and quote mismatch"
	}
	if quote.Sender == "" || quote.Receiver == "" || quote.MmAddr == "" {
		return false, "quote.Sender, quote.Receiver or quote.MmAddr is empty"
	}
	if time.Now().Unix() > quote.SrcDeadline {
		return false, "past src deadline"
	}
	if quote.DstDeadline < quote.SrcDeadline {
		return false, "dst deadline is earlier than src deadline"
	}
	if !quote.ValidateQuoteHash() {
		return false, "quote hahs mismatch"
	}
	return true, ""
}
