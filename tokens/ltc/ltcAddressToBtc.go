package ltc

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/btcsuite/btcd/chaincfg"
	bchaincfg "github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcutil"
	"github.com/ltcsuite/ltcutil/base58"
	"github.com/ltcsuite/ltcutil/bech32"
	"golang.org/x/crypto/ripemd160"
)

// ConvertLTCAddress decode ltc address and convert to BTC address
func (b *Bridge) ConvertLTCAddress(addr, net string) (address btcutil.Address, err error) {
	bchainConfig := &bchaincfg.MainNetParams
	lchainConfig := b.GetChainParams()
	// Bech32 encoded segwit addresses start with a human-readable part
	// (hrp) followed by '1'. For Bitcoin mainnet the hrp is "bc", and for
	// testnet it is "tb". If the address string has a prefix that matches
	// one of the prefixes for the known networks, we try to decode it as
	// a segwit address.
	oneIndex := strings.LastIndexByte(addr, '1')
	if oneIndex > 1 {
		prefix := addr[:oneIndex+1]
		if chaincfg.IsBech32SegwitPrefix(prefix) {
			witnessVer, witnessProg, err := decodeLTCSegWitAddress(addr)
			if err != nil {
				return nil, err
			}

			// We currently only support P2WPKH and P2WSH, which is
			// witness version 0.
			if witnessVer != 0 {
				return nil, btcutil.UnsupportedWitnessVerError(witnessVer)
			}

			switch len(witnessProg) {
			case 20:
				return btcutil.NewAddressWitnessPubKeyHash(witnessProg, bchainConfig)
			case 32:
				return btcutil.NewAddressWitnessScriptHash(witnessProg, bchainConfig)
			default:
				return nil, btcutil.UnsupportedWitnessProgLenError(len(witnessProg))
			}
		}
	}

	// Serialized public keys are either 65 bytes (130 hex chars) if
	// uncompressed/hybrid or 33 bytes (66 hex chars) if compressed.
	if len(addr) == 130 || len(addr) == 66 {
		serializedPubKey, err := hex.DecodeString(addr)
		if err != nil {
			return nil, err
		}
		return btcutil.NewAddressPubKey(serializedPubKey, bchainConfig)
	}

	// Switch on decoded length to determine the type.
	decoded, netID, err := base58.CheckDecode(addr)
	if err != nil {
		if err == base58.ErrChecksum {
			return nil, btcutil.ErrChecksumMismatch
		}
		return nil, errors.New("decoded address is of unknown format")
	}
	switch len(decoded) {
	case ripemd160.Size: // P2PKH or P2SH
		isP2PKH := netID == lchainConfig.PubKeyHashAddrID
		isP2SH := netID == lchainConfig.ScriptHashAddrID
		switch hash160 := decoded; {
		case isP2PKH && isP2SH:
			return nil, btcutil.ErrAddressCollision
		case isP2PKH:
			return btcutil.NewAddressPubKeyHash(hash160, bchainConfig)
		case isP2SH:
			return btcutil.NewAddressScriptHashFromHash(hash160, bchainConfig)
		default:
			return nil, btcutil.ErrUnknownAddressType
		}

	default:
		return nil, errors.New("decoded address is of unknown size")
	}
}

// decodeSegWitAddress parses a bech32 encoded segwit address string and
// returns the witness version and witness program byte representation.
func decodeLTCSegWitAddress(address string) (byte, []byte, error) {
	// Decode the bech32 encoded address.
	_, data, err := bech32.Decode(address)
	if err != nil {
		return 0, nil, err
	}

	// The first byte of the decoded address is the witness version, it must
	// exist.
	if len(data) < 1 {
		return 0, nil, fmt.Errorf("no witness version")
	}

	// ...and be <= 16.
	version := data[0]
	if version > 16 {
		return 0, nil, fmt.Errorf("invalid witness version: %v", version)
	}

	// The remaining characters of the address returned are grouped into
	// words of 5 bits. In order to restore the original witness program
	// bytes, we'll need to regroup into 8 bit words.
	regrouped, err := bech32.ConvertBits(data[1:], 5, 8, false)
	if err != nil {
		return 0, nil, err
	}

	// The regrouped data must be between 2 and 40 bytes.
	if len(regrouped) < 2 || len(regrouped) > 40 {
		return 0, nil, fmt.Errorf("invalid data length")
	}

	// For witness version 0, address MUST be exactly 20 or 32 bytes.
	if version == 0 && len(regrouped) != 20 && len(regrouped) != 32 {
		return 0, nil, fmt.Errorf("invalid data length for witness "+
			"version 0: %v", len(regrouped))
	}

	return version, regrouped, nil
}