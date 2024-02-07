package app

import (
	errorsmod "cosmossdk.io/errors"
	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/cosmos/cosmos-sdk/x/auth/ante"

	customante "github.com/dydxprotocol/v4-chain/protocol/app/ante"
	libante "github.com/dydxprotocol/v4-chain/protocol/lib/ante"
	clobante "github.com/dydxprotocol/v4-chain/protocol/x/clob/ante"
	clobtypes "github.com/dydxprotocol/v4-chain/protocol/x/clob/types"
)

// HandlerOptions are the options required for constructing an SDK AnteHandler.
// Note: This struct is defined here in order to add `ClobKeeper`. We use
// struct embedding to include the normal cosmos-sdk `HandlerOptions`.
type HandlerOptions struct {
	ante.HandlerOptions
	Codec        codec.Codec
	AuthStoreKey storetypes.StoreKey
	ClobKeeper   clobtypes.ClobKeeper
}

// NewAnteHandler returns an AnteHandler that checks and increments sequence
// numbers, checks signatures & account numbers, deducts fees from the first
// signer, and handles in-memory clob messages.
//
// Link to default `AnteHandler` used by cosmos sdk:
// https://github.com/cosmos/cosmos-sdk/blob/3bb27795742dab2451b232bab02b82566d1a0192/x/auth/ante/ante.go#L25
func NewAnteHandler(options HandlerOptions) (sdk.AnteHandler, error) {
	if options.AccountKeeper == nil {
		return nil, errorsmod.Wrap(sdkerrors.ErrLogic, "account keeper is required for ante builder")
	}

	if options.BankKeeper == nil {
		return nil, errorsmod.Wrap(sdkerrors.ErrLogic, "bank keeper is required for ante builder")
	}

	if options.ClobKeeper == nil {
		return nil, errorsmod.Wrap(sdkerrors.ErrLogic, "clob keeper is required for ante builder")
	}

	if options.SignModeHandler == nil {
		return nil, errorsmod.Wrap(sdkerrors.ErrLogic, "sign mode handler is required for ante builder")
	}

	anteDecorators := NewAnteDecoratorChain(options)
	return sdk.ChainAnteDecorators(anteDecorators...), nil
}

// NewAnteDecoratorChain returns a list of AnteDecorators in the expected application chain ordering
func NewAnteDecoratorChain(options HandlerOptions) []sdk.AnteDecorator {
	return []sdk.AnteDecorator{
		// DeliverTx is executing using a lock and a branched multistore ensuring that all invocations are handled
		// serially allowing for any reads and writes without needing to hold any additional locks.
		// For CheckTx we are executing with an UNBRANCHED multistore so it is critical that no writes are
		// performed as those writes will be directly to the check state and also will cause race conditions.

		// Note: app-injected messages, and clob transactions don't require Gas fees.
		libante.NewAppInjectedMsgAnteWrapper(
			clobante.NewSingleMsgClobTxAnteWrapper(
				ante.NewSetUpContextDecorator(), // outermost AnteDecorator. SetUpContext must be called first
			),
		),

		// Set `FreeInfiniteGasMeter` for app-injected messages, and clob transactions.
		customante.NewFreeInfiniteGasDecorator(),

		ante.NewExtensionOptionsDecorator(options.ExtensionOptionChecker),
		customante.NewValidateMsgTypeDecorator(),

		// Note: app-injected messages are not signed on purpose.
		libante.NewAppInjectedMsgAnteWrapper(
			ante.NewValidateBasicDecorator(),
		),

		ante.NewTxTimeoutHeightDecorator(),
		ante.NewValidateMemoDecorator(options.AccountKeeper),

		// For CLOB CheckTx we need to grab a lock over all accounts that are part of signing. This allows
		// us to read and write account information in a manner in which all account reads and writes are linearized.
		customante.NewLockAccountsAnteDecorator(options.Codec, options.AuthStoreKey),
		ante.NewConsumeGasForTxSizeDecorator(options.AccountKeeper),

		// Note: app-injected messages, and clob transactions don't require Gas fees. This is important
		// since we do not have any locks over the bank or fee keeper to linearize the reads and writes.
		libante.NewAppInjectedMsgAnteWrapper(
			clobante.NewSingleMsgClobTxAnteWrapper(
				ante.NewDeductFeeDecorator(
					options.AccountKeeper,
					options.BankKeeper,
					options.FeegrantKeeper,
					options.TxFeeChecker,
				),
			),
		),

		// SetPubKeyDecorator must be called before all signature verification decorators
		// Note: app-injected messages are not signed on purpose.
		libante.NewAppInjectedMsgAnteWrapper(
			ante.NewSetPubKeyDecorator(options.AccountKeeper),
		),

		ante.NewValidateSigCountDecorator(options.AccountKeeper),

		// Note: app-injected messages don't require Gas fees.
		libante.NewAppInjectedMsgAnteWrapper(
			ante.NewSigGasConsumeDecorator(options.AccountKeeper, options.SigGasConsumer),
		),

		// Note: app-injected messages are not signed on purpose.
		libante.NewAppInjectedMsgAnteWrapper(
			customante.NewSigVerificationDecorator(options.AccountKeeper, options.SignModeHandler),
		),

		// Note: app-injected messages, and short-term clob txs don't have sequence numbers on purpose.
		libante.NewAppInjectedMsgAnteWrapper(
			clobante.NewShortTermSingleMsgClobTxAnteWrapper(
				ante.NewIncrementSequenceDecorator(options.AccountKeeper),
			),
		),

		// At this point in time we grab a lock ensuring that the ante handlers below are processed
		// serially. Note that no AccountKeeper is used so we don't require any lock coarsening for
		// any accounts that could be involved with the CLOB message.
		customante.NewLockAnteDecorator(),
		clobante.NewRateLimitDecorator(options.ClobKeeper),
		clobante.NewClobDecorator(options.ClobKeeper),
	}
}
