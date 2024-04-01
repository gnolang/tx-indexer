package model

import (
	"encoding/json"

	"github.com/gnolang/gno/gno.land/pkg/sdk/vm"
	"github.com/gnolang/gno/tm2/pkg/sdk/bank"
	"github.com/gnolang/gno/tm2/pkg/std"
)

// Deref dereferences a pointer variable.
func Deref[T any](v *T) T {
	if v == nil {
		var zero T

		return zero
	}

	return *v
}

// Cast casts a type.
func Cast[T any](input any) (*T, error) {
	encoded, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}

	var data T
	if err := json.Unmarshal(encoded, &data); err != nil {
		return nil, err
	}

	return &data, nil
}

func ParseBankMsgSend(value std.Msg) BankMsgSend {
	decodedMessage, err := Cast[bank.MsgSend](value)
	if err != nil {
		return BankMsgSend{}
	}

	return BankMsgSend{
		FromAddress: decodedMessage.FromAddress.String(),
		ToAddress:   decodedMessage.ToAddress.String(),
		Amount:      decodedMessage.Amount.String(),
	}
}

func ParseVMMsgCall(value std.Msg) MsgCall {
	decodedMessage, err := Cast[vm.MsgCall](value)
	if err != nil {
		return MsgCall{}
	}

	return MsgCall{
		Caller:  decodedMessage.Caller.String(),
		Send:    decodedMessage.Send.String(),
		PkgPath: decodedMessage.PkgPath,
		Func:    decodedMessage.Func,
		Args:    decodedMessage.Args,
	}
}

func ParseVMAddPackage(value std.Msg) MsgAddPackage {
	decodedMessage, err := Cast[vm.MsgAddPackage](value)
	if err != nil {
		return MsgAddPackage{}
	}

	memFiles := make([]*MemFile, 0)
	for _, file := range decodedMessage.Package.Files {
		memFiles = append(memFiles, &MemFile{
			Name: file.Name,
			Body: file.Body,
		})
	}

	return MsgAddPackage{
		Creator: decodedMessage.Creator.String(),
		Package: &MemPackage{
			Name:  decodedMessage.Package.Name,
			Path:  decodedMessage.Package.Path,
			Files: memFiles,
		},
		Deposit: decodedMessage.Deposit.String(),
	}
}

func ParseVMMsgRun(value std.Msg) MsgRun {
	decodedMessage, err := Cast[vm.MsgRun](value)
	if err != nil {
		return MsgRun{}
	}

	memFiles := make([]*MemFile, 0)
	for _, file := range decodedMessage.Package.Files {
		memFiles = append(memFiles, &MemFile{
			Name: file.Name,
			Body: file.Body,
		})
	}

	return MsgRun{
		Caller: decodedMessage.Caller.String(),
		Send:   decodedMessage.Send.String(),
		Package: &MemPackage{
			Name:  decodedMessage.Package.Name,
			Path:  decodedMessage.Package.Path,
			Files: memFiles,
		},
	}
}
