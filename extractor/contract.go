/*

  Copyright 2017 Loopring Project Ltd (Loopring Foundation).

  Licensed under the Apache License, Version 2.0 (the "License");
  you may not use this file except in compliance with the License.
  You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

  Unless required by applicable law or agreed to in writing, software
  distributed under the License is distributed on an "AS IS" BASIS,
  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
  See the License for the specific language governing permissions and
  limitations under the License.

*/

package extractor

import (
	"fmt"
	"github.com/Loopring/relay/dao"
	"github.com/Loopring/relay/ethaccessor"
	"github.com/Loopring/relay/eventemiter"
	"github.com/Loopring/relay/log"
	"github.com/Loopring/relay/market/util"
	"github.com/Loopring/relay/types"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"math/big"
)

type EventData struct {
	Event           interface{}
	ContractAddress string // 某个合约具体地址
	TxHash          string // transaction hash
	CAbi            *abi.ABI
	Id              common.Hash
	Name            string
	BlockNumber     *big.Int
	Time            *big.Int
	Topics          []string
}

func newEventData(event *abi.Event, cabi *abi.ABI) EventData {
	var c EventData

	c.Id = event.Id()
	c.Name = event.Name
	c.CAbi = cabi

	return c
}

func (event *EventData) FullFilled(evtLog *ethaccessor.Log, blockTime *big.Int, txhash string) {
	event.Topics = evtLog.Topics
	event.BlockNumber = evtLog.BlockNumber.BigInt()
	event.Time = blockTime
	event.ContractAddress = evtLog.Address
	event.TxHash = txhash
}

type MethodData struct {
	Method          interface{}
	ContractAddress string // 某个合约具体地址
	From            string
	To              string
	TxHash          string // transaction hash
	CAbi            *abi.ABI
	Id              string
	Name            string
	BlockNumber     *big.Int
	Time            *big.Int
	Value           *big.Int
	Input           string
	LogAmount       int
	Gas             *big.Int
	GasPrice        *big.Int
}

func newMethodData(method *abi.Method, cabi *abi.ABI) MethodData {
	var c MethodData

	c.Id = common.ToHex(method.Id())
	c.Name = method.Name
	c.CAbi = cabi

	return c
}

func (method *MethodData) FullFilled(tx *ethaccessor.Transaction, blockTime *big.Int, logAmount int) {
	method.BlockNumber = tx.BlockNumber.BigInt()
	method.Time = blockTime
	method.ContractAddress = tx.To
	method.From = tx.From
	method.To = tx.To
	method.TxHash = tx.Hash
	method.Value = tx.Value.BigInt()
	method.BlockNumber = tx.BlockNumber.BigInt() //blockNumber
	method.Input = tx.Input
	method.Gas = tx.Gas.BigInt()
	method.GasPrice = tx.GasPrice.BigInt()
	method.LogAmount = logAmount
}

func (m *MethodData) IsValid() error {
	if m.LogAmount < 1 {
		return fmt.Errorf("method %s transaction logs == 0", m.Name)
	}
	return nil
}

const (
	RINGMINED_EVT_NAME           = "RingMined"
	CANCEL_EVT_NAME              = "OrderCancelled"
	CUTOFF_EVT_NAME              = "CutoffTimestampChanged"
	TRANSFER_EVT_NAME            = "Transfer"
	APPROVAL_EVT_NAME            = "Approval"
	TOKENREGISTERED_EVT_NAME     = "TokenRegistered"
	TOKENUNREGISTERED_EVT_NAME   = "TokenUnregistered"
	RINGHASHREGISTERED_EVT_NAME  = "RinghashSubmitted"
	ADDRESSAUTHORIZED_EVT_NAME   = "AddressAuthorized"
	ADDRESSDEAUTHORIZED_EVT_NAME = "AddressDeauthorized"

	SUBMITRING_METHOD_NAME          = "submitRing"
	CANCELORDER_METHOD_NAME         = "cancelOrder"
	SUBMITRINGHASH_METHOD_NAME      = "submitRinghash"
	BATCHSUBMITRINGHASH_METHOD_NAME = "batchSubmitRinghash"

	WETH_DEPOSIT_METHOD_NAME    = "deposit"
	WETH_WITHDRAWAL_METHOD_NAME = "withdraw"

	APPROVAL_METHOD_NAME = "approve"
)

type AbiProcessor struct {
	accessor  *ethaccessor.EthNodeAccessor
	events    map[common.Hash]EventData
	methods   map[string]MethodData
	protocols map[common.Address]string
	delegates map[common.Address]string
	db        dao.RdsService
}

// 这里无需考虑版本问题，对解析来说，不接受版本升级带来数据结构变化的可能性
func newAbiProcessor(accessor *ethaccessor.EthNodeAccessor, db dao.RdsService) *AbiProcessor {
	processor := &AbiProcessor{}

	processor.events = make(map[common.Hash]EventData)
	processor.methods = make(map[string]MethodData)
	processor.protocols = make(map[common.Address]string)
	processor.delegates = make(map[common.Address]string)
	processor.accessor = accessor
	processor.db = db

	processor.loadProtocolAddress()
	processor.loadErc20Contract()
	processor.loadWethContract()
	processor.loadProtocolContract()
	processor.loadTokenRegisterContract()
	processor.loadRingHashRegisteredContract()
	processor.loadTokenTransferDelegateProtocol()

	return processor
}

// GetEvent get EventData with id hash
func (processor *AbiProcessor) GetEvent(id common.Hash) (EventData, bool) {
	var (
		event EventData
		ok    bool
	)
	event, ok = processor.events[id]
	return event, ok
}

// GetMethod get MethodData with method id
func (processor *AbiProcessor) GetMethod(id string) (MethodData, bool) {
	var (
		method MethodData
		ok     bool
	)
	method, ok = processor.methods[id]
	return method, ok
}

// HasContract judge protocol have ever been load
func (processor *AbiProcessor) HasContract(protocol common.Address) bool {
	_, ok := processor.protocols[protocol]
	return ok
}

// HasSpender check approve spender address have ever been load
func (processor *AbiProcessor) HasSpender(spender common.Address) bool {
	_, ok := processor.delegates[spender]
	return ok
}

func (processor *AbiProcessor) loadProtocolAddress() {
	for _, v := range util.AllTokens {
		processor.protocols[v.Protocol] = v.Symbol
		log.Infof("extractor,contract protocol %s->%s", v.Symbol, v.Protocol.Hex())
	}

	for _, v := range processor.accessor.ProtocolAddresses {
		protocolSymbol := "loopring"
		delegateSymbol := "transfer_delegate"
		ringhashRegisterSymbol := "ringhash_register"
		tokenRegisterSymbol := "token_register"

		processor.protocols[v.ContractAddress] = protocolSymbol
		processor.protocols[v.TokenRegistryAddress] = tokenRegisterSymbol
		processor.protocols[v.RinghashRegistryAddress] = ringhashRegisterSymbol
		processor.protocols[v.DelegateAddress] = delegateSymbol

		processor.delegates[v.DelegateAddress] = delegateSymbol

		log.Infof("extractor,contract protocol %s->%s", protocolSymbol, v.ContractAddress.Hex())
		log.Infof("extractor,contract protocol %s->%s", tokenRegisterSymbol, v.TokenRegistryAddress.Hex())
		log.Infof("extractor,contract protocol %s->%s", ringhashRegisterSymbol, v.RinghashRegistryAddress.Hex())
		log.Infof("extractor,contract protocol %s->%s", delegateSymbol, v.DelegateAddress.Hex())
	}
}

func (processor *AbiProcessor) loadProtocolContract() {
	for name, event := range processor.accessor.ProtocolImplAbi.Events {
		if name != RINGMINED_EVT_NAME && name != CANCEL_EVT_NAME && name != CUTOFF_EVT_NAME {
			continue
		}

		watcher := &eventemitter.Watcher{}
		contract := newEventData(&event, processor.accessor.ProtocolImplAbi)

		switch contract.Name {
		case RINGMINED_EVT_NAME:
			contract.Event = &ethaccessor.RingMinedEvent{}
			watcher = &eventemitter.Watcher{Concurrent: false, Handle: processor.handleRingMinedEvent}
		case CANCEL_EVT_NAME:
			contract.Event = &ethaccessor.OrderCancelledEvent{}
			watcher = &eventemitter.Watcher{Concurrent: false, Handle: processor.handleOrderCancelledEvent}
		case CUTOFF_EVT_NAME:
			contract.Event = &ethaccessor.CutoffTimestampChangedEvent{}
			watcher = &eventemitter.Watcher{Concurrent: false, Handle: processor.handleCutoffTimestampEvent}
		}

		eventemitter.On(contract.Id.Hex(), watcher)
		processor.events[contract.Id] = contract
		log.Infof("extractor,contract event name:%s -> key:%s", contract.Name, contract.Id.Hex())
	}

	for name, method := range processor.accessor.ProtocolImplAbi.Methods {
		if name != SUBMITRING_METHOD_NAME && name != CANCELORDER_METHOD_NAME {
			continue
		}

		contract := newMethodData(&method, processor.accessor.ProtocolImplAbi)
		watcher := &eventemitter.Watcher{}

		switch contract.Name {
		case SUBMITRING_METHOD_NAME:
			contract.Method = &ethaccessor.SubmitRingMethod{}
			watcher = &eventemitter.Watcher{Concurrent: false, Handle: processor.handleSubmitRingMethod}
		case CANCELORDER_METHOD_NAME:
			contract.Method = &ethaccessor.CancelOrderMethod{}
			watcher = &eventemitter.Watcher{Concurrent: false, Handle: processor.handleCancelOrderMethod}
		}

		eventemitter.On(contract.Id, watcher)
		processor.methods[contract.Id] = contract
		log.Infof("extractor,contract method name:%s -> key:%s", contract.Name, contract.Id)
	}
}

func (processor *AbiProcessor) loadErc20Contract() {
	for name, event := range processor.accessor.Erc20Abi.Events {
		if name != TRANSFER_EVT_NAME && name != APPROVAL_EVT_NAME {
			continue
		}

		watcher := &eventemitter.Watcher{}
		contract := newEventData(&event, processor.accessor.Erc20Abi)

		switch contract.Name {
		case TRANSFER_EVT_NAME:
			contract.Event = &ethaccessor.TransferEvent{}
			watcher = &eventemitter.Watcher{Concurrent: false, Handle: processor.handleTransferEvent}
		case APPROVAL_EVT_NAME:
			contract.Event = &ethaccessor.ApprovalEvent{}
			watcher = &eventemitter.Watcher{Concurrent: false, Handle: processor.handleApprovalEvent}
		}

		eventemitter.On(contract.Id.Hex(), watcher)
		processor.events[contract.Id] = contract
		log.Infof("extractor,contract event name:%s -> key:%s", contract.Name, contract.Id.Hex())
	}

	for name, method := range processor.accessor.Erc20Abi.Methods {
		if name != APPROVAL_METHOD_NAME {
			continue
		}

		contract := newMethodData(&method, processor.accessor.Erc20Abi)
		contract.Method = &ethaccessor.ApproveMethod{}
		watcher := &eventemitter.Watcher{Concurrent: false, Handle: processor.handleApproveMethod}
		eventemitter.On(contract.Id, watcher)

		processor.methods[contract.Id] = contract

		log.Infof("extractor,contract method name:%s -> key:%s", contract.Name, contract.Id)
	}
}

func (processor *AbiProcessor) loadWethContract() {
	for name, method := range processor.accessor.WethAbi.Methods {
		if name != WETH_DEPOSIT_METHOD_NAME && name != WETH_WITHDRAWAL_METHOD_NAME {
			continue
		}

		watcher := &eventemitter.Watcher{}
		contract := newMethodData(&method, processor.accessor.WethAbi)

		switch contract.Name {
		case WETH_DEPOSIT_METHOD_NAME:
			// weth deposit without any inputs,use transaction.value as input
			watcher = &eventemitter.Watcher{Concurrent: false, Handle: processor.handleWethDepositMethod}
		case WETH_WITHDRAWAL_METHOD_NAME:
			contract.Method = &ethaccessor.WethWithdrawalMethod{}
			watcher = &eventemitter.Watcher{Concurrent: false, Handle: processor.handleWethWithdrawalMethod}
		}

		eventemitter.On(contract.Id, watcher)
		processor.methods[contract.Id] = contract
		log.Infof("extractor,contract method name:%s -> key:%s", contract.Name, contract.Id)
	}
}

func (processor *AbiProcessor) loadTokenRegisterContract() {
	for name, event := range processor.accessor.TokenRegistryAbi.Events {
		if name != TOKENREGISTERED_EVT_NAME && name != TOKENUNREGISTERED_EVT_NAME {
			continue
		}

		watcher := &eventemitter.Watcher{}
		contract := newEventData(&event, processor.accessor.TokenRegistryAbi)

		switch contract.Name {
		case TOKENREGISTERED_EVT_NAME:
			contract.Event = &ethaccessor.TokenRegisteredEvent{}
			watcher = &eventemitter.Watcher{Concurrent: false, Handle: processor.handleTokenRegisteredEvent}
		case TOKENUNREGISTERED_EVT_NAME:
			contract.Event = &ethaccessor.TokenUnRegisteredEvent{}
			watcher = &eventemitter.Watcher{Concurrent: false, Handle: processor.handleTokenUnRegisteredEvent}
		}

		eventemitter.On(contract.Id.Hex(), watcher)
		processor.events[contract.Id] = contract
		log.Infof("extractor,contract event name:%s -> key:%s", contract.Name, contract.Id.Hex())
	}
}

func (processor *AbiProcessor) loadRingHashRegisteredContract() {
	for name, event := range processor.accessor.RinghashRegistryAbi.Events {
		if name != RINGHASHREGISTERED_EVT_NAME {
			continue
		}

		contract := newEventData(&event, processor.accessor.RinghashRegistryAbi)
		contract.Event = &ethaccessor.RingHashSubmittedEvent{}

		watcher := &eventemitter.Watcher{Concurrent: false, Handle: processor.handleRinghashSubmitEvent}
		eventemitter.On(contract.Id.Hex(), watcher)

		processor.events[contract.Id] = contract
		log.Infof("extractor,contract event name:%s -> key:%s", contract.Name, contract.Id.Hex())
	}

	for name, method := range processor.accessor.RinghashRegistryAbi.Methods {
		if name != BATCHSUBMITRINGHASH_METHOD_NAME && name != SUBMITRINGHASH_METHOD_NAME {
			continue
		}

		contract := newMethodData(&method, processor.accessor.RinghashRegistryAbi)
		watcher := &eventemitter.Watcher{}

		switch contract.Name {
		case SUBMITRINGHASH_METHOD_NAME:
			contract.Method = &ethaccessor.SubmitRingHashMethod{}
			watcher = &eventemitter.Watcher{Concurrent: false, Handle: processor.handleSubmitRingHashMethod}
		case BATCHSUBMITRINGHASH_METHOD_NAME:
			contract.Method = &ethaccessor.BatchSubmitRingHashMethod{}
			watcher = &eventemitter.Watcher{Concurrent: false, Handle: processor.handleBatchSubmitRingHashMethod}
		}

		eventemitter.On(contract.Id, watcher)
		processor.methods[contract.Id] = contract
		log.Infof("extractor,contract method name:%s -> key:%s", contract.Name, contract.Id)
	}
}

func (processor *AbiProcessor) loadTokenTransferDelegateProtocol() {
	for name, event := range processor.accessor.DelegateAbi.Events {
		if name != ADDRESSAUTHORIZED_EVT_NAME && name != ADDRESSDEAUTHORIZED_EVT_NAME {
			continue
		}

		watcher := &eventemitter.Watcher{}
		contract := newEventData(&event, processor.accessor.DelegateAbi)

		switch contract.Name {
		case ADDRESSAUTHORIZED_EVT_NAME:
			contract.Event = &ethaccessor.AddressAuthorizedEvent{}
			watcher = &eventemitter.Watcher{Concurrent: false, Handle: processor.handleAddressAuthorizedEvent}
		case ADDRESSDEAUTHORIZED_EVT_NAME:
			contract.Event = &ethaccessor.AddressDeAuthorizedEvent{}
			watcher = &eventemitter.Watcher{Concurrent: false, Handle: processor.handleAddressDeAuthorizedEvent}
		}

		eventemitter.On(contract.Id.Hex(), watcher)
		processor.events[contract.Id] = contract
		log.Infof("extractor,contract event name:%s -> key:%s", contract.Name, contract.Id.Hex())
	}
}

// 只需要解析submitRing,cancel，cutoff这些方法在event里，如果方法不成功也不用执行后续逻辑
func (processor *AbiProcessor) handleSubmitRingMethod(input eventemitter.EventData) error {
	contract := input.(MethodData)

	// emit to miner
	var evt types.SubmitRingMethodEvent
	evt.TxHash = common.HexToHash(contract.TxHash)
	evt.UsedGas = contract.Gas
	evt.UsedGasPrice = contract.GasPrice
	evt.Err = contract.IsValid()

	log.Debugf("extractor,submitRing method,tx:%s, gas:%s, gasprice:%s", evt.TxHash.Hex(), evt.UsedGas.String(), evt.UsedGasPrice.String())

	eventemitter.Emit(eventemitter.Miner_SubmitRing_Method, &evt)

	ring := contract.Method.(*ethaccessor.SubmitRingMethod)
	ring.Protocol = common.HexToAddress(contract.To)

	data := hexutil.MustDecode("0x" + contract.Input[10:])
	if err := contract.CAbi.UnpackMethodInput(ring, contract.Name, data); err != nil {
		log.Errorf("extractor,submitRing method,tx:%s, unpack error:%s", evt.TxHash.Hex(), err.Error())
		return nil
	}
	orderList, err := ring.ConvertDown()
	if err != nil {
		log.Errorf("extractor,submitRing method,tx:%s convert order data error:%s", evt.TxHash.Hex(), err.Error())
		return nil
	}

	for _, v := range orderList {
		v.Protocol = common.HexToAddress(contract.ContractAddress)
		v.Hash = v.GenerateHash()
		log.Debugf("extractor,submitRing method,tx:%s orderHash:%s,owner:%s,tokenS:%s,tokenB:%s,amountS:%s,amountB:%s", evt.TxHash.Hex(), v.Hash.Hex(), v.Owner.Hex(), v.TokenS.Hex(), v.TokenB.Hex(), v.AmountS.String(), v.AmountB.String())
		eventemitter.Emit(eventemitter.Gateway, v)
	}

	return nil
}

func (processor *AbiProcessor) handleSubmitRingHashMethod(input eventemitter.EventData) error {
	contract := input.(MethodData)
	method := contract.Method.(*ethaccessor.SubmitRingHashMethod)

	data := hexutil.MustDecode("0x" + contract.Input[10:])
	if err := contract.CAbi.UnpackMethodInput(method, contract.Name, data); err != nil {
		log.Errorf("extractor,submitRingHash method,tx:%s unpack error:%s", contract.TxHash, err.Error())
		return nil
	}
	evt, err := method.ConvertDown()
	if err != nil {
		log.Errorf("extractor,submitRingHash method,tx:%s convert order data error:%s", contract.TxHash, err.Error())
		return nil
	}

	evt.TxHash = common.HexToHash(contract.TxHash)
	evt.UsedGas = contract.Gas
	evt.UsedGasPrice = contract.GasPrice
	evt.Err = contract.IsValid()

	log.Debugf("extractor,submitRingHash method,tx:%s, gas:%s, gasprice:%s", evt.TxHash.Hex(), evt.UsedGas.String(), evt.UsedGasPrice.String())

	eventemitter.Emit(eventemitter.Miner_SubmitRingHash_Method, evt)

	return nil
}

func (processor *AbiProcessor) handleBatchSubmitRingHashMethod(input eventemitter.EventData) error {
	contract := input.(MethodData)
	method := contract.Method.(*ethaccessor.BatchSubmitRingHashMethod)

	data := hexutil.MustDecode("0x" + contract.Input[10:])
	if err := contract.CAbi.UnpackMethodInput(method, contract.Name, data); err != nil {
		log.Errorf("extractor,batchSubmitRingHash method,tx:%s unpack error:%s", contract.TxHash, err.Error())
		return nil
	}
	evt, err := method.ConvertDown()
	if err != nil {
		log.Errorf("extractor,batchSubmitRingHash method,tx:%s convert order data error:%s", contract.TxHash, err.Error())
		return nil
	}

	evt.TxHash = common.HexToHash(contract.TxHash)
	evt.UsedGas = contract.Gas
	evt.UsedGasPrice = contract.GasPrice
	evt.Err = contract.IsValid()

	log.Debugf("extractor,batchSubmitRingHash method,tx:%s, gas:%s, gasprice:%s", evt.TxHash.Hex(), evt.UsedGas.String(), evt.UsedGasPrice.String())

	eventemitter.Emit(eventemitter.Miner_BatchSubmitRingHash_Method, evt)

	return nil
}

func (processor *AbiProcessor) handleCancelOrderMethod(input eventemitter.EventData) error {
	contract := input.(MethodData)
	cancel := contract.Method.(*ethaccessor.CancelOrderMethod)

	data := hexutil.MustDecode("0x" + contract.Input[10:])
	if err := contract.CAbi.UnpackMethodInput(cancel, contract.Name, data); err != nil {
		log.Errorf("extractor,cancelOrder method,tx:%s unpack error:%s", contract.TxHash, err.Error())
		return nil
	}

	order, err := cancel.ConvertDown()
	if err != nil {
		log.Errorf("extractor,cancelOrder method,tx:%s convert order data error:%s", contract.TxHash, err.Error())
		return nil
	}

	log.Debugf("extractor,cancelOrder method,tx:%s, order tokenS:%s,tokenB:%s,amountS:%s,amountB:%s", contract.TxHash, order.TokenS.Hex(), order.TokenB.Hex(), order.AmountS.String(), order.AmountB.String())

	order.Protocol = common.HexToAddress(contract.ContractAddress)
	eventemitter.Emit(eventemitter.Gateway, order)

	return nil
}

func (processor *AbiProcessor) handleApproveMethod(input eventemitter.EventData) error {
	contractData := input.(MethodData)
	contractMethod := contractData.Method.(*ethaccessor.ApproveMethod)

	data := hexutil.MustDecode("0x" + contractData.Input[10:])
	if err := contractData.CAbi.UnpackMethodInput(contractMethod, contractData.Name, data); err != nil {
		log.Errorf("extractor,approve method,tx:%s unpack error:%s", contractData.TxHash, err.Error())
		return nil
	}

	approve := contractMethod.ConvertDown()
	approve.Owner = common.HexToAddress(contractData.From)
	approve.From = common.HexToAddress(contractData.From)
	approve.To = common.HexToAddress(contractData.To)
	approve.Time = contractData.Time
	approve.Blocknumber = contractData.BlockNumber
	approve.TxHash = common.HexToHash(contractData.TxHash)
	approve.ContractAddress = common.HexToAddress(contractData.ContractAddress)

	log.Debugf("extractor,approve method, tx:%s, owner:%s, spender:%s, value:%s", contractData.TxHash, approve.Owner.Hex(), approve.Spender.Hex(), approve.Value.String())

	if processor.HasSpender(approve.Spender) {
		eventemitter.Emit(eventemitter.ApproveMethod, approve)
	}

	return nil
}

func (processor *AbiProcessor) handleWethDepositMethod(input eventemitter.EventData) error {
	contractData := input.(MethodData)

	var deposit types.WethDepositMethodEvent
	deposit.From = common.HexToAddress(contractData.From)
	deposit.To = common.HexToAddress(contractData.To)
	deposit.Value = contractData.Value
	deposit.Time = contractData.Time
	deposit.Blocknumber = contractData.BlockNumber
	deposit.TxHash = common.HexToHash(contractData.TxHash)
	deposit.ContractAddress = common.HexToAddress(contractData.ContractAddress)

	log.Debugf("extractor,wethDeposit method,tx:%s, from:%s, to:%s, value:%s", contractData.TxHash, deposit.From.Hex(), deposit.To.Hex(), deposit.Value.String())

	eventemitter.Emit(eventemitter.WethDepositMethod, &deposit)
	return nil
}

func (processor *AbiProcessor) handleWethWithdrawalMethod(input eventemitter.EventData) error {
	contractData := input.(MethodData)
	contractMethod := contractData.Method.(*ethaccessor.WethWithdrawalMethod)

	data := hexutil.MustDecode("0x" + contractData.Input[10:])
	if err := contractData.CAbi.UnpackMethodInput(&contractMethod.Value, contractData.Name, data); err != nil {
		log.Errorf("extractor,wethWithdrawal method,tx:%s unpack error:%s", contractData.TxHash, err.Error())
		return nil
	}

	withdrawal := contractMethod.ConvertDown()
	withdrawal.From = common.HexToAddress(contractData.From)
	withdrawal.To = common.HexToAddress(contractData.To)
	withdrawal.Time = contractData.Time
	withdrawal.Blocknumber = contractData.BlockNumber
	withdrawal.TxHash = common.HexToHash(contractData.TxHash)
	withdrawal.ContractAddress = common.HexToAddress(contractData.ContractAddress)

	log.Debugf("extractor,wethWithdrawal method,tx:%s, from:%s, to:%s, value:%s", contractData.TxHash, withdrawal.From.Hex(), withdrawal.To.Hex(), withdrawal.Value.String())

	eventemitter.Emit(eventemitter.WethWithdrawalMethod, withdrawal)
	return nil
}

func (processor *AbiProcessor) handleRingMinedEvent(input eventemitter.EventData) error {
	contractData := input.(EventData)
	if len(contractData.Topics) < 2 {
		log.Errorf("extractor,ringmined event,tx:%s indexed fields number error", contractData.TxHash)
		return nil
	}

	contractEvent := contractData.Event.(*ethaccessor.RingMinedEvent)
	contractEvent.RingHash = common.HexToHash(contractData.Topics[1])

	ringmined, fills, err := contractEvent.ConvertDown()
	if err != nil {
		log.Errorf("extractor,ringmined event,tx:%s convert down error:%s", contractData.TxHash, err.Error())
		return nil
	}
	ringmined.ContractAddress = common.HexToAddress(contractData.ContractAddress)
	ringmined.TxHash = common.HexToHash(contractData.TxHash)
	ringmined.Time = contractData.Time
	ringmined.Blocknumber = contractData.BlockNumber

	log.Debugf("extractor,ring mined event,tx:%s, ringhash:%s, ringIndex:%s, tx:%s",
		contractData.TxHash,
		ringmined.Ringhash.Hex(),
		ringmined.RingIndex.String(),
		ringmined.TxHash.Hex())

	eventemitter.Emit(eventemitter.OrderManagerExtractorRingMined, ringmined)

	var (
		fillList      []*types.OrderFilledEvent
		orderhashList []string
	)
	for _, fill := range fills {
		fill.TxHash = common.HexToHash(contractData.TxHash)
		fill.ContractAddress = common.HexToAddress(contractData.ContractAddress)
		fill.Time = contractData.Time
		fill.Blocknumber = contractData.BlockNumber

		log.Debugf("extractor,order filled event,tx:%s, ringhash:%s, amountS:%s, amountB:%s, orderhash:%s, lrcFee:%s, lrcReward:%s, nextOrderhash:%s, preOrderhash:%s, ringIndex:%s",
			contractData.TxHash,
			fill.Ringhash.Hex(),
			fill.AmountS.String(),
			fill.AmountB.String(),
			fill.OrderHash.Hex(),
			fill.LrcFee.String(),
			fill.LrcReward.String(),
			fill.NextOrderHash.Hex(),
			fill.PreOrderHash.Hex(),
			fill.RingIndex.String(),
		)

		fillList = append(fillList, fill)
		orderhashList = append(orderhashList, fill.OrderHash.Hex())
	}

	ordermap, err := processor.db.GetOrdersByHash(orderhashList)
	if err != nil {
		log.Errorf("extractor,ringmined event,tx:%s getOrdersByHash error:%s", contractData.TxHash, err.Error())
		return nil
	}

	for _, v := range fillList {
		if ord, ok := ordermap[v.OrderHash.Hex()]; ok {
			v.TokenS = common.HexToAddress(ord.TokenS)
			v.TokenB = common.HexToAddress(ord.TokenB)
			v.Owner = common.HexToAddress(ord.Owner)
			v.Market, _ = util.WrapMarketByAddress(v.TokenB.Hex(), v.TokenS.Hex())
			eventemitter.Emit(eventemitter.OrderManagerExtractorFill, v)
		} else {
			log.Debugf("extractor,orderFilled event,tx:%s cann't match order %s", contractData.TxHash, ord.OrderHash)
		}
	}

	return nil
}

func (processor *AbiProcessor) handleOrderCancelledEvent(input eventemitter.EventData) error {
	contractData := input.(EventData)
	if len(contractData.Topics) < 2 {
		log.Errorf("extractor,order cancelled event,tx:%s indexed fields number error", contractData.TxHash)
		return nil
	}

	contractEvent := contractData.Event.(*ethaccessor.OrderCancelledEvent)
	contractEvent.OrderHash = common.HexToHash(contractData.Topics[1])

	evt := contractEvent.ConvertDown()
	evt.TxHash = common.HexToHash(contractData.TxHash)
	evt.ContractAddress = common.HexToAddress(contractData.ContractAddress)
	evt.Time = contractData.Time
	evt.Blocknumber = contractData.BlockNumber

	log.Debugf("extractor,order cancelled event,tx:%s, orderhash:%s, cancelAmount:%s", contractData.TxHash, evt.OrderHash.Hex(), evt.AmountCancelled.String())

	eventemitter.Emit(eventemitter.OrderManagerExtractorCancel, evt)

	return nil
}

func (processor *AbiProcessor) handleCutoffTimestampEvent(input eventemitter.EventData) error {
	contractData := input.(EventData)
	if len(contractData.Topics) < 2 {
		log.Errorf("extractor,cutoffTimestampChanged event,tx:%s indexed fields number error", contractData.TxHash)
		return nil
	}

	contractEvent := contractData.Event.(*ethaccessor.CutoffTimestampChangedEvent)
	contractEvent.Owner = common.HexToAddress(contractData.Topics[1])

	evt := contractEvent.ConvertDown()
	evt.TxHash = common.HexToHash(contractData.TxHash)
	evt.ContractAddress = common.HexToAddress(contractData.ContractAddress)
	evt.Time = contractData.Time
	evt.Blocknumber = contractData.BlockNumber

	log.Debugf("extractor,cutoffTimestampChanged event,tx:%s, ownerAddress:%s, cutOffTime:%s", contractData.TxHash, evt.Owner.Hex(), evt.Cutoff.String())

	eventemitter.Emit(eventemitter.OrderManagerExtractorCutoff, evt)

	return nil
}

func (processor *AbiProcessor) handleTransferEvent(input eventemitter.EventData) error {
	contractData := input.(EventData)

	if len(contractData.Topics) < 3 {
		log.Errorf("extractor,tokenTransfer event,tx:%s indexed fields number error", contractData.TxHash)
		return nil
	}

	contractEvent := contractData.Event.(*ethaccessor.TransferEvent)
	contractEvent.From = common.HexToAddress(contractData.Topics[1])
	contractEvent.To = common.HexToAddress(contractData.Topics[2])

	evt := contractEvent.ConvertDown()
	evt.ContractAddress = common.HexToAddress(contractData.ContractAddress)
	evt.Time = contractData.Time
	evt.Blocknumber = contractData.BlockNumber

	log.Debugf("extractor,transfer event,tx:%s, from:%s, to:%s, value:%s", contractData.TxHash, evt.From.Hex(), evt.To.Hex(), evt.Value.String())

	eventemitter.Emit(eventemitter.AccountTransfer, evt)

	return nil
}

func (processor *AbiProcessor) handleApprovalEvent(input eventemitter.EventData) error {
	contractData := input.(EventData)
	if len(contractData.Topics) < 3 {
		log.Errorf("extractor,approval event,tx:%s indexed fields number error", contractData.TxHash)
		return nil
	}

	contractEvent := contractData.Event.(*ethaccessor.ApprovalEvent)
	contractEvent.Owner = common.HexToAddress(contractData.Topics[1])
	contractEvent.Spender = common.HexToAddress(contractData.Topics[2])

	evt := contractEvent.ConvertDown()
	evt.ContractAddress = common.HexToAddress(contractData.ContractAddress)
	evt.Time = contractData.Time
	evt.Blocknumber = contractData.BlockNumber

	log.Debugf("extractor,approval event,tx:%s, owner:%s, spender:%s, value:%s", contractData.TxHash, evt.Owner.Hex(), evt.Spender.Hex(), evt.Value.String())

	if processor.HasSpender(evt.Spender) {
		eventemitter.Emit(eventemitter.AccountApproval, evt)
	}

	return nil
}

func (processor *AbiProcessor) handleTokenRegisteredEvent(input eventemitter.EventData) error {
	contractData := input.(EventData)
	contractEvent := contractData.Event.(*ethaccessor.TokenRegisteredEvent)

	evt := contractEvent.ConvertDown()
	evt.ContractAddress = common.HexToAddress(contractData.ContractAddress)
	evt.Time = contractData.Time
	evt.Blocknumber = contractData.BlockNumber

	log.Debugf("extractor,token registered event,tx:%s, address:%s, symbol:%s", contractData.TxHash, evt.Token.Hex(), evt.Symbol)

	eventemitter.Emit(eventemitter.TokenRegistered, evt)

	return nil
}

func (processor *AbiProcessor) handleTokenUnRegisteredEvent(input eventemitter.EventData) error {
	contractData := input.(EventData)
	contractEvent := contractData.Event.(*ethaccessor.TokenUnRegisteredEvent)

	evt := contractEvent.ConvertDown()
	evt.ContractAddress = common.HexToAddress(contractData.ContractAddress)
	evt.Time = contractData.Time
	evt.Blocknumber = contractData.BlockNumber

	log.Debugf("extractor,token unregistered event,tx:%s, address:%s, symbol:%s", contractData.TxHash, evt.Token.Hex(), evt.Symbol)

	eventemitter.Emit(eventemitter.TokenUnRegistered, evt)

	return nil
}

func (processor *AbiProcessor) handleRinghashSubmitEvent(input eventemitter.EventData) error {
	contractData := input.(EventData)
	if len(contractData.Topics) < 3 {
		log.Errorf("extractor,ringhash registered event,tx:%s indexed fields number error", contractData.TxHash)
		return nil
	}

	contractEvent := contractData.Event.(*ethaccessor.RingHashSubmittedEvent)
	contractEvent.RingMiner = common.HexToAddress(contractData.Topics[1])
	contractEvent.RingHash = common.HexToHash(contractData.Topics[2])

	evt := contractEvent.ConvertDown()
	evt.ContractAddress = common.HexToAddress(contractData.ContractAddress)
	evt.Time = contractData.Time
	evt.Blocknumber = contractData.BlockNumber
	evt.TxHash = common.HexToHash(contractData.TxHash)

	log.Debugf("extractor,ringhash submit event,tx:%s, ringhash:%s, ringMiner:%s", contractData.TxHash, evt.RingHash.Hex(), evt.RingMiner.Hex())

	eventemitter.Emit(eventemitter.RingHashSubmitted, evt)

	return nil
}

func (processor *AbiProcessor) handleAddressAuthorizedEvent(input eventemitter.EventData) error {
	contractData := input.(EventData)
	if len(contractData.Topics) < 2 {
		log.Errorf("extractor,address authorized event,tx:%s indexed fields number error", contractData.TxHash)
		return nil
	}

	contractEvent := contractData.Event.(*ethaccessor.AddressAuthorizedEvent)
	contractEvent.ContractAddress = common.HexToAddress(contractData.Topics[1])

	evt := contractEvent.ConvertDown()
	evt.ContractAddress = common.HexToAddress(contractData.ContractAddress)
	evt.Time = contractData.Time
	evt.Blocknumber = contractData.BlockNumber

	log.Debugf("extractor,address authorized event,tx:%s, address:%s, number:%d", contractData.TxHash, evt.Protocol.Hex(), evt.Number)

	eventemitter.Emit(eventemitter.AddressAuthorized, evt)

	return nil
}

func (processor *AbiProcessor) handleAddressDeAuthorizedEvent(input eventemitter.EventData) error {
	contractData := input.(EventData)
	if len(contractData.Topics) < 2 {
		log.Errorf("extractor,address deauthorized event,tx:%s indexed fields number error", contractData.TxHash)
		return nil
	}

	contractEvent := contractData.Event.(*ethaccessor.AddressDeAuthorizedEvent)
	contractEvent.ContractAddress = common.HexToAddress(contractData.Topics[1])

	evt := contractEvent.ConvertDown()
	evt.ContractAddress = common.HexToAddress(contractData.ContractAddress)
	evt.Time = contractData.Time
	evt.Blocknumber = contractData.BlockNumber

	log.Debugf("extractor,address deauthorized event,tx:%s, address:%s, number:%d", contractData.TxHash, evt.Protocol.Hex(), evt.Number)

	eventemitter.Emit(eventemitter.AddressAuthorized, evt)

	return nil
}
