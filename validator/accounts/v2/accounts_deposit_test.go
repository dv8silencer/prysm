package v2

import (
	"encoding/hex"
	"errors"
	"flag"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/prysmaticlabs/prysm/shared/bytesutil"
	"github.com/prysmaticlabs/prysm/shared/testutil/assert"
	"github.com/prysmaticlabs/prysm/shared/testutil/require"
	"github.com/prysmaticlabs/prysm/validator/flags"
	v2keymanager "github.com/prysmaticlabs/prysm/validator/keymanager/v2"
	"github.com/prysmaticlabs/prysm/validator/keymanager/v2/derived"
	"github.com/urfave/cli/v2"
)

type depositTestWalletConfig struct {
	walletDir                   string
	walletPasswordFile          string
	eth1KeystoreFile            string
	eth1KeystorePasswordFile    string
	eth1PrivateKeyFile          string
	httpWeb3ProviderFlag        string
	publicKeysFlag              string
	depositAllAccountsFlag      bool
	skipDepositConfirmationFlag bool
	keymanagerKind              v2keymanager.Kind
}

func setupWalletCtxforDeposits(
	t *testing.T,
	cfg *depositTestWalletConfig,
) *cli.Context {
	app := cli.App{}
	set := flag.NewFlagSet("test", 0)
	set.String(flags.WalletDirFlag.Name, cfg.walletDir, "")
	set.String(flags.KeymanagerKindFlag.Name, cfg.keymanagerKind.String(), "")
	set.String(flags.WalletPasswordFileFlag.Name, cfg.walletPasswordFile, "")
	set.String(flags.HTTPWeb3ProviderFlag.Name, cfg.httpWeb3ProviderFlag, "")
	set.String(flags.Eth1KeystoreUTCPathFlag.Name, cfg.eth1KeystoreFile, "")
	set.String(flags.Eth1KeystorePasswordFileFlag.Name, cfg.eth1KeystorePasswordFile, "")
	set.String(flags.Eth1PrivateKeyFileFlag.Name, cfg.eth1PrivateKeyFile, "")
	set.String(flags.DepositPublicKeysFlag.Name, cfg.publicKeysFlag, "")
	set.Bool(flags.DepositAllAccountsFlag.Name, cfg.depositAllAccountsFlag, "")
	set.Bool(flags.SkipDepositConfirmationFlag.Name, cfg.skipDepositConfirmationFlag, "")
	assert.NoError(t, set.Set(flags.WalletDirFlag.Name, cfg.walletDir))
	assert.NoError(t, set.Set(flags.KeymanagerKindFlag.Name, cfg.keymanagerKind.String()))
	assert.NoError(t, set.Set(flags.WalletPasswordFileFlag.Name, cfg.walletPasswordFile))
	assert.NoError(t, set.Set(flags.HTTPWeb3ProviderFlag.Name, cfg.httpWeb3ProviderFlag))
	if cfg.eth1KeystoreFile != "" {
		assert.NoError(t, set.Set(flags.Eth1KeystoreUTCPathFlag.Name, cfg.eth1KeystoreFile))
		assert.NoError(t, set.Set(flags.Eth1KeystorePasswordFileFlag.Name, cfg.eth1KeystorePasswordFile))
	}
	if cfg.eth1PrivateKeyFile != "" {
		assert.NoError(t, set.Set(flags.Eth1PrivateKeyFileFlag.Name, cfg.eth1PrivateKeyFile))
	}
	if cfg.publicKeysFlag != "" {
		assert.NoError(t, set.Set(flags.DepositPublicKeysFlag.Name, cfg.publicKeysFlag))
	}
	if cfg.depositAllAccountsFlag == true {
		assert.NoError(t, set.Set(flags.DepositAllAccountsFlag.Name, strconv.FormatBool(cfg.depositAllAccountsFlag)))
	}
	assert.NoError(t, set.Set(flags.SkipDepositConfirmationFlag.Name, strconv.FormatBool(cfg.skipDepositConfirmationFlag)))
	return cli.NewContext(&app, set, nil)
}

func TestCreateDepositConfig(t *testing.T) {
	walletDir, _, passwordFilePath := setupWalletAndPasswordsDir(t)

	// First, create the wallet and several accounts
	cliCtx := setupWalletCtx(t, &testWalletConfig{
		keymanagerKind:     v2keymanager.Derived,
		walletDir:          walletDir,
		walletPasswordFile: passwordFilePath,
		skipDepositConfirm: true,
	})
	wallet, err := CreateAndSaveWalletCli(cliCtx)
	require.NoError(t, err)

	err = CreateAccount(cliCtx.Context, &CreateAccountConfig{
		Wallet:      wallet,
		NumAccounts: 3,
	})
	require.NoError(t, err)
	keymanager, err := wallet.InitializeKeymanager(
		cliCtx.Context,
		true, /* skip mnemonic confirm */
	)
	require.NoError(t, err)

	// Save public keys for comparison and selection purposes later
	pubkeys, err := keymanager.FetchValidatingPublicKeys(cliCtx.Context)
	require.NoError(t, err)
	var hexPubkeys []string
	for _, pubkey := range pubkeys {
		encoded := make([]byte, hex.EncodedLen(len(pubkey)))
		hex.Encode(encoded, pubkey[:])
		hexPubkeys = append(hexPubkeys, string(encoded))
	}
	// Remove the last key so that we can select a portion and not all of the accounts later
	hexPubkeys = hexPubkeys[:len(hexPubkeys)-1]
	hexPubkeysString := strings.Join(hexPubkeys, ",")

	// Create a file holding the test ETH1 private key
	eth1PrivateKeyFile, err := ioutil.TempFile("", "testing")
	require.NoError(t, err)
	defer func() {
		err = eth1PrivateKeyFile.Close()
		require.NoError(t, err)
		err = os.Remove(eth1PrivateKeyFile.Name())
		require.NoError(t, err)
	}()
	_, err = eth1PrivateKeyFile.WriteString("This should be an ETH1 private key")
	require.NoError(t, err)

	// First we test the behavior when depositAllAccountsFlag is set to true
	depositConfig, err := createDepositConfigHelper(t, &depositTestWalletConfig{
		keymanagerKind:              v2keymanager.Derived,
		walletDir:                   walletDir,
		walletPasswordFile:          passwordFilePath,
		skipDepositConfirmationFlag: true,
		depositAllAccountsFlag:      true,
		httpWeb3ProviderFlag:        "http://localhost:8545",
		eth1PrivateKeyFile:          eth1PrivateKeyFile.Name(),
	})
	require.NoError(t, err)

	require.Equal(t, 3, len(depositConfig.DepositPublicKeys), "wrong number of public keys")

	if depositConfig.Eth1PrivateKey != "This should be an ETH1 private key" {
		require.NoError(t, errors.New("eth1 private key does not match"))
	}
	if depositConfig.Web3Provider != "http://localhost:8545" {
		require.NoError(t, errors.New("web3 provider does not match"))
	}
	if depositConfig.Eth1KeystoreUTCFile != "" || depositConfig.Eth1KeystorePasswordFile != "" {
		require.NoError(t, errors.New("keystore file and keystore password file paths should be empty"))
	}

	// Test the case of providing the public keys via command-line.  We also pass in the test eth1 private key file.
	// hexPubkeysString holds 1 less than all the accounts.
	depositConfig, err = createDepositConfigHelper(t, &depositTestWalletConfig{
		keymanagerKind:              v2keymanager.Derived,
		walletDir:                   walletDir,
		walletPasswordFile:          passwordFilePath,
		skipDepositConfirmationFlag: true,
		publicKeysFlag:              hexPubkeysString,
		httpWeb3ProviderFlag:        "http://localhost:8545",
		eth1PrivateKeyFile:          eth1PrivateKeyFile.Name(),
	})
	require.NoError(t, err)

	require.Equal(t, 2, len(depositConfig.DepositPublicKeys), "wrong number of public keys")

	// Compare the keys in the config object with the keys we obtained earlier from the keymanager
	for keyNum, configPubKey := range depositConfig.DepositPublicKeys {
		for index, eachByte := range bytesutil.ToBytes48(configPubKey.Marshal()) {
			if eachByte != pubkeys[keyNum][index] {
				require.NoError(t, errors.New("public keys do not match"))
			}
		}
	}

	// Now we test when private key file is not provided but rather the keystore and keystore password files
	depositConfig, err = createDepositConfigHelper(t, &depositTestWalletConfig{
		keymanagerKind:              v2keymanager.Derived,
		walletDir:                   walletDir,
		walletPasswordFile:          passwordFilePath,
		skipDepositConfirmationFlag: true,
		depositAllAccountsFlag:      true,
		httpWeb3ProviderFlag:        "http://localhost:8545",
		eth1KeystoreFile:            "This would be eth1 keystore file path",
		eth1KeystorePasswordFile:    "This would be eth1 keystore password file path",
	})
	require.NoError(t, err)

	require.Equal(t, 3, len(depositConfig.DepositPublicKeys), "wrong number of public keys")

	if depositConfig.Eth1KeystoreUTCFile != "This would be eth1 keystore file path" ||
		depositConfig.Eth1KeystorePasswordFile != "This would be eth1 keystore password file path" {
		require.NoError(t, errors.New("eth1 keystore or keystore password file path incorrect"))
	}
	if depositConfig.Eth1PrivateKey != "" {
		require.NoError(t, errors.New("eth1 private key should be empty string"))
	}
}

// createDepositConfigHelper returns a SendDepositConfig when given a particular wallet configuration.
func createDepositConfigHelper(t *testing.T, config *depositTestWalletConfig) (*derived.SendDepositConfig, error) {
	cliCtx := setupWalletCtxforDeposits(t, config)
	wallet, err := OpenWalletOrElseCli(cliCtx, func(cliCtx *cli.Context) (*Wallet, error) {
		err := errors.New("could not open wallet")
		require.NoError(t, err)
		return nil, err
	})
	require.NoError(t, err)

	keymanager, err := wallet.InitializeKeymanager(
		cliCtx.Context,
		true, /* skip mnemonic confirm */
	)
	require.NoError(t, err)
	km, ok := keymanager.(*derived.Keymanager)
	if !ok {
		log.Fatalf("keymanager must be derived type")
	}

	// Now we finally call the function we are testing.
	depositConfig, err := createDepositConfig(cliCtx, km)
	require.NoError(t, err)
	return depositConfig, nil
}
