package smart_contract_test

import (
	"context"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/kurtosis-tech/avalanche-smart-contract-sample-testsuite/smart_contracts/bindings"
	"github.com/kurtosis-tech/avalanche-smart-contract-sample-testsuite/testsuite/networks_impl"
	"github.com/kurtosis-tech/kurtosis-libs/golang/lib/networks"
	"github.com/kurtosis-tech/kurtosis-libs/golang/lib/testsuite"
	"github.com/palantir/stacktrace"
	"github.com/sirupsen/logrus"
	"math/big"
	"time"
)

const (
	maxNumCheckTransactionMinedRetries = 10
	timeBetweenCheckTransactionMinedRetries = 1 * time.Second
)

type SmartContractTest struct {
	avalancheImage string
}

func NewSmartContractTest(avalancheImage string) *SmartContractTest {
	return &SmartContractTest{avalancheImage: avalancheImage}
}

func (test SmartContractTest) Configure(builder *testsuite.TestConfigurationBuilder) {
	builder.WithSetupTimeoutSeconds(180).WithRunTimeoutSeconds(180)
}

func (test *SmartContractTest) Setup(networkCtx *networks.NetworkContext) (networks.Network, error) {
	network := networks_impl.NewSmartContractAvalancheNetwork(test.avalancheImage, networkCtx)
	if err := network.SetupAvalancheNetwork(); err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred setting up the Avalanche network")
	}
	return network, nil
}

func (test SmartContractTest) Run(uncastedNetwork networks.Network) error {
	// Necessary because Go doesn't have generics
	network, ok := uncastedNetwork.(*networks_impl.SmartContractAvalancheNetwork)
	if !ok {
		return stacktrace.NewError("Couldn't cast the generic network to the appropriate type")
	}
	gethClient, transactor := network.GetFundedCChainClientAndTransactor()

	// TODO vvvvvvvvvvvvvvvvvvvvvvvv REPLACE WITH YOUR CUSTOM TEST CODE vvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvv
	logrus.Info("Deploying HelloWorld contract...")
	_, helloWorldDeploymentTxn, _, err := bindings.DeployHelloWorld(transactor, gethClient)
	if err != nil {
		return stacktrace.Propagate(err, "An error occurred deploying the HelloWorld contract on the C-Chain")
	}
	if err := waitUntilTransactionMined(gethClient, helloWorldDeploymentTxn.Hash()); err != nil {
		return stacktrace.Propagate(err, "An error occurred waiting for the HelloWorld contract deployment transaction to be mined")
	}
	logrus.Info("HelloWorld contract deployed")

	logrus.Info("Deploying SimpleStorage contract...")
	_, storageDeploymentTxn, storageContract, err := bindings.DeploySimpleStorage(transactor, gethClient)
	if err != nil {
		return stacktrace.Propagate(err, "An error occurred deploying the SimpleStorage contract on the C-Chain")
	}
	if err := waitUntilTransactionMined(gethClient, storageDeploymentTxn.Hash()); err != nil {
		return stacktrace.Propagate(err, "An error occurred waiting for the SimpleStorage contract deployment transaction to be mined")
	}
	// NOTE: It's not clear why we need to sleep here - the transaction being mined should be sufficient
	time.Sleep(5 * time.Second)
	logrus.Info("SimpleStorage contract deployed")

	valueToStore := big.NewInt(20)
	logrus.Infof("Storing value '%v'...", valueToStore)
	storeValueTxn, err := storageContract.Set(transactor, valueToStore)
	if err != nil {
		return stacktrace.Propagate(err, "An error occurred storing value '%v' in the contract", valueToStore)
	}
	if err := waitUntilTransactionMined(gethClient, storeValueTxn.Hash()); err != nil {
		return stacktrace.Propagate(err, "An error occurred waiting for the value-storing transaction to be mined")
	}
	// NOTE: It's not clear why we need to sleep here - the transaction being mined should be sufficient
	time.Sleep(5 * time.Second)
	logrus.Info("Value stored")

	logrus.Info("Retrieving value from contract...")
	retrievedValue, err := storageContract.Get(&bind.CallOpts{})
	if err != nil {
		return stacktrace.Propagate(err, "An error occurred retrieving the value stored in the contract")
	}
	logrus.Infof("Retrieved value: %v", retrievedValue)

	if valueToStore.Cmp(retrievedValue) != 0 {
		return stacktrace.NewError("Retrieved value '%v' != stored value '%v'", retrievedValue, valueToStore)
	}
	// TODO ^^^^^^^^^^^^^^^^^^^^^^^^ REPLACE WITH YOUR CUSTOM TEST CODE ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

	return nil
}


// If we try to use a contract immediately after submission without waiting for it to be mined, we'll get a "no contract code at address" error:
// https://github.com/ethereum/go-ethereum/issues/15930#issuecomment-532144875
func waitUntilTransactionMined(validatorClient *ethclient.Client, transactionHash common.Hash) error {
	for i := 0; i < maxNumCheckTransactionMinedRetries; i++ {
		receipt, err := validatorClient.TransactionReceipt(context.Background(), transactionHash)
		if err == nil && receipt != nil && receipt.BlockNumber != nil {
			return nil
		}
		if i < maxNumCheckTransactionMinedRetries - 1 {
			time.Sleep(timeBetweenCheckTransactionMinedRetries)
		}
	}
	return stacktrace.NewError(
		"Transaction with hash '%v' wasn't mined even after checking %v times with %v between checks",
		transactionHash.Hex(),
		maxNumCheckTransactionMinedRetries,
		timeBetweenCheckTransactionMinedRetries)
}
