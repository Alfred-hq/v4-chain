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

var _ sdk.AnteDecorator = lockAccountsAnteDecorator{}
var _ sdk.AnteDecorator = lockAnteDecorator{}

func NewLockAnteDecorator() sdk.AnteDecorator {
	return lockAnteDecorator{}
}

func NewLockAccountsAnteDecorator(cdc codec.Codec, authStoreKey storetypes.StoreKey) sdk.AnteDecorator {
	return lockAccountsAnteDecorator{
		cdc:             cdc,
		authStoreKey:    authStoreKey,
		lockAndCacheCtx: NewLockAnteDecorator(),
	}
}

type lockAccountsAnteDecorator struct {
	cdc             codec.Codec
	authStoreKey    storetypes.StoreKey
	lockAndCacheCtx sdk.AnteDecorator
}

func (l lockAccountsAnteDecorator) AnteHandle(
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
		return l.lockAndCacheCtx.AnteHandle(ctx, tx, simulate, next)
	}

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

	newCtx, err := next(ctx.WithMultiStore(cacheMs), tx, simulate)
	if err == nil {
		cacheMs.Write()
	}
	return newCtx, err
}

type lockAnteDecorator struct {
	mtx sync.Mutex
}

func (l lockAnteDecorator) AnteHandle(ctx sdk.Context, tx sdk.Tx, simulate bool, next sdk.AnteHandler) (sdk.Context, error) {
	l.mtx.Lock()
	defer l.mtx.Unlock()

	return next(ctx, tx, simulate)
}
