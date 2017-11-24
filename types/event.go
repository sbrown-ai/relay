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

package types

import "github.com/ethereum/go-ethereum/common"

type TokenRegisterEvent struct {
	Token       common.Address
	Symbol      string
	Blocknumber *Big
	Time        *Big
}

type TokenUnRegisterEvent struct {
	Token       common.Address
	Symbol      string
	Blocknumber *Big
	Time        *Big
}

type RinghashSubmittedEvent struct {
	RingHash    common.Hash
	RingMiner   common.Address
	Blocknumber *Big
	Time        *Big
}

// todo: unpack transaction and create event
type EtherBalanceUpdateEvent struct {
	Owner common.Address
}

// todo: transfer change to
type TokenBalanceUpdateEvent struct {
	Owner       common.Address
	Value       *Big
	BlockNumber *Big
	BlockHash   common.Hash
}

// todo: erc20 event
type TokenAllowanceUpdateEvent struct {
	Owner       common.Address
	Spender     common.Address
	Value       *Big
	BlockNumber *Big
	BlockHash   common.Hash
}

type TransferEvent struct {
	From        common.Address
	To          common.Address
	Value       *Big
	Blocknumber *Big
	Time        *Big
}

type ApprovalEvent struct {
	Owner       common.Address
	Spender     common.Address
	Value       *Big
	Blocknumber *Big
	Time        *Big
}

type OrderFilledEvent struct {
	Ringhash      common.Hash
	PreOrderHash  common.Hash
	OrderHash     common.Hash
	NextOrderHash common.Hash
	RingIndex     *Big
	Time          *Big
	Blocknumber   *Big
	AmountS       *Big
	AmountB       *Big
	LrcReward     *Big
	LrcFee        *Big
	IsDeleted     bool
}

type OrderCancelledEvent struct {
	OrderHash       common.Hash
	Time            *Big
	Blocknumber     *Big
	AmountCancelled *Big
	IsDeleted       bool
}

type CutoffEvent struct {
	Owner       common.Address
	Time        *Big
	Blocknumber *Big
	Cutoff      *Big
	IsDeleted   bool
}

type RingMinedEvent struct {
	RingIndex          *Big
	Time               *Big
	Blocknumber        *Big
	Ringhash           common.Hash
	Miner              common.Address
	FeeRecipient       common.Address
	IsRinghashReserved bool
	IsDeleted          bool
}

type SubmitRingEvent struct {
	TxHash common.Hash
}

type RingHashRegistryEvent struct {
	RingHash common.Hash
	RingMiner common.Address
	TxHash common.Hash
}