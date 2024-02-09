package ante

import (
	"cosmossdk.io/collections"
	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/store/cachemulti"
	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	clobante "github.com/dydxprotocol/v4-chain/protocol/x/clob/ante"
	"sync"
)

var _ sdk.AnteDecorator = accountLockAnteDecorator{}
var _ sdk.AnteDecorator = nonClobLockAnteDecorator{}

func NewLockingAnteDecorators(cdc codec.Codec, authStoreKey storetypes.StoreKey) (writeAndUnlock sdk.AnteDecorator, selectiveLocking sdk.AnteDecorator, clobLocking sdk.AnteDecorator) {
	mtx := &sync.Mutex{}
	return writeAndUnlockAnteDecorator{
			mtx: mtx,
		},
		accountLockAnteDecorator{
			cdc:          cdc,
			authStoreKey: authStoreKey,
		},
		nonClobLockAnteDecorator{
			mtx: mtx,
		}
}

type accountLockAnteDecorator struct {
	cdc          codec.Codec
	authStoreKey storetypes.StoreKey
	mtx          *sync.Mutex
}

func (l accountLockAnteDecorator) AnteHandle(
	ctx sdk.Context,
	tx sdk.Tx,
	simulate bool,
	next sdk.AnteHandler,
) (sdk.Context, error) {
	isClob, err := clobante.IsSingleClobMsgTx(ctx, tx)
	if err != nil {
		return ctx, err
	}

	if !isClob {
		// Acquire a global lock for all non-CLOB messages.
		cacheMs := ctx.MultiStore().(cachemulti.Store).CacheMultiStore()
		l.mtx.Lock()
		return next(ctx.WithMultiStore(cacheMs), tx, simulate)
	}

	// For CLOB messages grab a lock for the account store for the keys that are part of signing.
	sigTx, ok := tx.(authsigning.SigVerifiableTx)
	if !ok {
		return ctx, errorsmod.Wrap(sdkerrors.ErrTxDecode, "Tx must be a sigTx")
	}
	signers, err := sigTx.GetSigners()
	if err != nil {
		return ctx, err
	}

	accountStoreKeys := make([][]byte, len(signers))
	for i, signer := range signers {
		encodedSigner, err := collections.EncodeKeyWithPrefix(authtypes.AddressStoreKeyPrefix, sdk.AccAddressKey, signer)
		if err != nil {
			return ctx, err
		}
		accountStoreKeys[i] = encodedSigner
	}

	cacheMs := ctx.MultiStore().(cachemulti.Store).CacheMultiStoreWithLocking(map[storetypes.StoreKey][][]byte{
		l.authStoreKey: accountStoreKeys,
	})

	return next(ctx.WithMultiStore(cacheMs), tx, simulate)
}

type nonClobLockAnteDecorator struct {
	mtx *sync.Mutex
}

func (l nonClobLockAnteDecorator) AnteHandle(ctx sdk.Context, tx sdk.Tx, simulate bool, next sdk.AnteHandler) (sdk.Context, error) {
	isClob, err := clobante.IsSingleClobMsgTx(ctx, tx)

	// We only have CLOB decorators after this so we can return early here since they are no-ops for non-CLOB messages.
	if err != nil || !isClob {
		return ctx, err
	}

	// Only acquire the global lock if this is a CLOB message since non CLOB messages would have already
	// acquired a global lock earlier.
	l.mtx.Lock()

	return next(ctx, tx, simulate)
}

type writeAndUnlockAnteDecorator struct {
	mtx *sync.Mutex
}

func (l writeAndUnlockAnteDecorator) AnteHandle(ctx sdk.Context, tx sdk.Tx, simulate bool, next sdk.AnteHandler) (sdk.Context, error) {
	newCtx, err := next(ctx, tx, simulate)
	if err == nil {
		newCtx.MultiStore().(storetypes.CacheMultiStore).Write()
	}

	l.mtx.Unlock()
	return newCtx, err
}
