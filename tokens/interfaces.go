package tokens

import (
	"errors"
	"math/big"
)

const (
	LockMemoPrefix   = "SWAPTO:"
	UnlockMemoPrefix = "SWAPTX:"
	RecallMemoPrefix = "RECALL:"
)

var (
	SrcBridge CrossChainBridge
	DstBridge CrossChainBridge

	// first 4 bytes of `Keccak256Hash([]byte("Swapin(bytes32,address,uint256)"))`
	SwapinFuncHash = [4]byte{0xec, 0x12, 0x6c, 0x77}
)

var (
	ErrBridgeSourceNotSupported      = errors.New("bridge source not supported")
	ErrBridgeDestinationNotSupported = errors.New("bridge destination not supported")

	ErrTodo = errors.New("developing: TODO")

	ErrTxNotStable         = errors.New("tx not stable")
	ErrTxWithWrongValue    = errors.New("tx with wrong value")
	ErrTxWithWrongReceiver = errors.New("tx with wrong receiver")
	ErrTxWithWrongMemo     = errors.New("tx with wrong memo")
	ErrTxWithWrongStatus   = errors.New("tx with wrong status")
	ErrTxWithWrongReceipt  = errors.New("tx with wrong receipt")
)

type TokenConfig struct {
	BlockChain      *string
	NetID           *string
	ID              string
	Name            string
	Symbol          string
	Decimals        *uint8
	Description     string
	DcrmAddress     *string
	ContractAddress *string
	Confirmations   *uint64
	MaximumSwap     *float64 // whole unit (eg. BTC, ETH, FSN), not Satoshi
	MinimumSwap     *float64 // whole unit
	SwapFeeRate     *float64
}

func (c *TokenConfig) CheckConfig(isSrc bool) error {
	if c.BlockChain == nil || *c.BlockChain == "" {
		return errors.New("token must config 'BlockChain'")
	}
	if c.NetID == nil || *c.NetID == "" {
		return errors.New("token must config 'NetID'")
	}
	if c.Decimals == nil {
		return errors.New("token must config 'Decimals'")
	}
	if c.Confirmations == nil {
		return errors.New("token must config 'Confirmations'")
	}
	if c.MaximumSwap == nil {
		return errors.New("token must config 'MaximumSwap'")
	}
	if c.MinimumSwap == nil {
		return errors.New("token must config 'MinimumSwap'")
	}
	if c.SwapFeeRate == nil {
		return errors.New("token must config 'SwapFeeRate'")
	}
	if c.DcrmAddress == nil || *c.DcrmAddress == "" {
		return errors.New("token must config 'DcrmAddress'")
	}
	if !isSrc && (c.ContractAddress == nil || *c.ContractAddress == "") {
		return errors.New("token must config 'ContractAddress' for destination chain")
	}
	return nil
}

type GatewayConfig struct {
	ApiAddress string
}

type SwapType uint32

const (
	Swap_Swapin SwapType = iota
	Swap_Swapout
	Swap_Recall
)

type SwapInfo struct {
	TxHash   string   `json:"txhash"`
	SwapType SwapType `json:"swaptype"`
	Extra    string   `json:"extra,omitempty"` // record extra info to verify msghash (eg. gas price)
}

type TxSwapInfo struct {
	Hash      string `json:"hash"`
	Height    uint64 `json:"height"`
	Timestamp uint64 `json:"timestamp"`
	From      string `json:"from"`
	To        string `json:"to"`
	Bind      string `json:"bind"`
	Value     string `json:"value"`
}

type TxStatus struct {
	Confirmations uint64 `json:"confirmations"`
	Block_height  uint64 `json:"block_height"`
	Block_hash    string `json:"block_hash"`
	Block_time    uint64 `json:"block_time"`
}

type BuildTxArgs struct {
	IsSwapin      bool     `json:"isSwapin,omitempty"`
	From          string   `json:"from"`
	To            string   `json:"to"`
	Value         *big.Int `json:"value"`
	Memo          string   `json:"memo,omitempty"`
	Gas           *uint64  `json:"gas,omitempty"`           // eth
	GasPrice      *big.Int `json:"gasPrice,omitempty"`      // eth
	Nonce         *uint64  `json:"nonce,omitempty"`         // eth
	Input         *[]byte  `json:"input,omitempty"`         // eth erc20 ...
	FeeRate       *int64   `json:"feeRate,omitempty"`       // btc
	ChangeAddress *string  `json:"changeAddress,omitempty"` // btc
	FromPublicKey *string  `json:"fromPublickey,omitempty"` // btc
}

type CrossChainBridge interface {
	IsSrcEndpoint() bool
	GetTokenAndGateway() (*TokenConfig, *GatewayConfig)
	SetTokenAndGateway(*TokenConfig, *GatewayConfig)

	IsValidAddress(address string) bool

	GetTransactionStatus(txHash string) *TxStatus
	VerifyTransaction(txHash string) (*TxSwapInfo, error)

	BuildRawTransaction(args *BuildTxArgs) (rawTx interface{}, err error)
	DcrmSignTransaction(rawTx, swapInfo interface{}) (signedTx interface{}, err error)
	SendTransaction(signedTx interface{}) (txHash string, err error)
}

type CrossChainBridgeBase struct {
	TokenConfig   *TokenConfig
	GatewayConfig *GatewayConfig
	IsSrc         bool
}

func NewCrossChainBridgeBase(isSrc bool) *CrossChainBridgeBase {
	return &CrossChainBridgeBase{IsSrc: isSrc}
}

func (b *CrossChainBridgeBase) IsSrcEndpoint() bool {
	return b.IsSrc
}

func (b *CrossChainBridgeBase) GetTokenAndGateway() (*TokenConfig, *GatewayConfig) {
	return b.TokenConfig, b.GatewayConfig
}

func (b *CrossChainBridgeBase) SetTokenAndGateway(tokenCfg *TokenConfig, gatewayCfg *GatewayConfig) {
	b.TokenConfig = tokenCfg
	b.GatewayConfig = gatewayCfg
	err := tokenCfg.CheckConfig(true)
	if err != nil {
		panic(err)
	}
}

func GetTokenConfig(isSrc bool) *TokenConfig {
	var token *TokenConfig
	if isSrc {
		token, _ = SrcBridge.GetTokenAndGateway()
	} else {
		token, _ = DstBridge.GetTokenAndGateway()
	}
	return token
}

func CheckSwapValue(value float64, isSrc bool) bool {
	token := GetTokenConfig(isSrc)
	return value >= *token.MinimumSwap && value <= *token.MaximumSwap
}

func CalcSwappedValue(value *big.Int, isSrc bool) *big.Int {
	token := GetTokenConfig(isSrc)

	swapFeeRate := new(big.Float).SetFloat64(*token.SwapFeeRate)
	swapValue := new(big.Float).SetInt(value)
	swapFee := new(big.Float).Mul(swapValue, swapFeeRate)

	swappedValue := new(big.Float).Sub(swapValue, swapFee)

	result := big.NewInt(0)
	swappedValue.Int(result)
	return result
}