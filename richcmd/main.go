package main

import (
	"context"
	"database/sql"
	"encoding/hex"
	"slices"
	"strings"
	"time"

	_ "embed"

	"github.com/glebarez/sqlite"
	_ "github.com/gnolang/gno/gno.land/pkg/gnoland"
	"github.com/gnolang/gno/gno.land/pkg/sdk/vm"
	"github.com/gnolang/gno/gnovm/pkg/gnolang"
	"github.com/gnolang/gno/tm2/pkg/amino"
	_ "github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/gnolang/gno/tm2/pkg/std"
	tm2 "github.com/gnolang/gno/tm2/pkg/std"
	"github.com/hasura/go-graphql-client"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"golang.org/x/crypto/sha3"
	"gorm.io/gorm"
)

//go:embed duplicate_realms.sql
var duplicateRealmsSQL string

//go:embed names.sql
var namesSQL string

func main() {
	logger, err := zap.NewDevelopment()
	if err != nil {
		panic(errors.Wrap(err, "failed to create logger"))
	}

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		panic(errors.Wrap(err, "failed to open db"))
	}
	if err := db.AutoMigrate(allModels...); err != nil {
		panic(errors.Wrap(err, "failed to sync db schema"))
	}

	client := graphql.NewClient("http://localhost:8546/graphql/query", nil)

	lastHeight := int64(0)
	lastIndex := int64(-1)
	limit := 10000

	for newTxs := true; newTxs; {
		nextIndex := lastIndex + 1

		logger.Info("query", zap.Int64("lastHeight", lastHeight), zap.Int64("nextIndex", nextIndex))

		var txQuery struct {
			Transactions []struct {
				BlockHeight int64 `graphql:"block_height"`
				Index       int64
				Code        int
				ContentRaw  string `graphql:"content_raw"`
			} `graphql:"transactions(filter: { from_block_height: $fromHeight, from_index: $fromIndex, limit: $limit })"`
		}

		// FIXME: need to filter txs that succeeded
		if err := client.Query(context.TODO(), &txQuery, map[string]any{
			"fromHeight": lastHeight,
			"fromIndex":  nextIndex,
			"limit":      limit,
		}); err != nil {
			panic(errors.Wrap(err, "failed to query"))
		}

		for _, tx := range txQuery.Transactions {
			if tx.Code != 0 {
				logger.Debug("ignored tx", zap.Int64("height", tx.BlockHeight), zap.Int64("index", tx.Index))
				continue
			}

			txBytes, err := hex.DecodeString(tx.ContentRaw[3 : len(tx.ContentRaw)-1])
			if err != nil {
				panic(errors.Wrap(err, "failed to decode tx"))
			}
			var tm2Tx tm2.Tx
			if err := amino.Unmarshal(txBytes, &tm2Tx); err != nil {
				panic(errors.Wrap(err, "failed to unmarshal tx"))
			}
			for _, msg := range tm2Tx.GetMsgs() {
				switch msg.Route() {
				case "vm":
					switch msg.Type() {
					case "exec":
						msg := msg.(vm.MsgCall)
						// logger.Debug("vm.exec", zap.String("type", msg.Type()), zap.String("realm", msg.PkgPath))
						switch msg.PkgPath {
						case "gno.land/r/demo/users":
							switch msg.Func {
							case "Register":
								name := msg.Args[1]
								addr := msg.Caller.String()
								if err := db.Save(&User{Name: name, Address: addr}).Error; err != nil {
									panic(errors.Wrap(err, "failed to save user"))
								}
								logger.Debug("maybe registered user", zap.String("name", name), zap.String("address", addr))
							default:
								// logger.Debug("vm.exec", zap.String("type", msg.Type()), zap.String("realm", msg.PkgPath), zap.String("func", msg.Func), zap.Any("args", msg.Args))
							}
						}
						continue
					case "add_package":
						msg := msg.(vm.MsgAddPackage)
						addr := gnolang.DerivePkgAddr(msg.Package.Path)
						codeHash, err := hashCode(msg.Package.Files)
						if err != nil {
							panic(errors.Wrap(err, "failed to hash code"))
						}
						realm := &Realm{
							Address:     addr.String(),
							PackagePath: msg.Package.Path,
							CodeHash:    codeHash,
						}
						if err := db.Save(realm).Error; err != nil {
							panic(errors.Wrap(err, "failed to save realm"))
						}
						logger.Debug("maybe instantiated realm", zap.String("package", msg.Package.Path), zap.String("code_hash", hex.EncodeToString(codeHash)), zap.String("address", addr.String()))
						// logger.Debug("indexed message", zap.String("fqtype", "vm.add_package"), zap.Any("realm", realm))
						continue
					case "run":
						// msg := msg.(vm.MsgRun)
						// logger.Debug("vm.run", zap.String("type", msg.Type()), zap.String("pkg", msg.Package.Path))
						continue
					}
				case "bank":
					switch msg.Type() {
					case "send":
						// msg := msg.(bank.MsgSend)
						// logger.Debug("bank.send", zap.String("type", msg.Type()), zap.String("from", msg.FromAddress.String()), zap.String("to", msg.ToAddress.String()), zap.Any("amount", msg.Amount))
						continue
					}
				}
				logger.Debug("unknown", zap.String("route", msg.Route()), zap.String("type", msg.Type()), zap.Any("msg", msg))
			}

			lastHeight = tx.BlockHeight
			lastIndex = int64(tx.Index)
		}

		if len(txQuery.Transactions) <= 0 {
			newTxs = false
		} else {
			time.Sleep(2 * time.Second) // would be better to use a sub
		}
	}

	var results []queryResult
	if err := db.Raw(duplicateRealmsSQL).Scan(&results).Error; err != nil {
		panic(errors.Wrap(err, "failed to query"))
	}
	for _, result := range results {
		paths := strings.Split(result.Paths, ",")
		logger.Debug("maybe found duplicate code", zap.String("code_hash", result.CodeHash), zap.Int("count", result.Count), zap.Any("paths", paths))
	}

	var users []User
	if err := db.Raw(namesSQL, sql.Named("search", "u")).Scan(&users).Error; err != nil {
		panic(errors.Wrap(err, "failed to query"))
	}
	for _, user := range users {
		logger.Debug("maybe found user with u", zap.String("name", user.Name), zap.String("address", user.Address))
	}
}

type queryResult struct {
	CodeHash string
	Count    int
	Paths    string
}

func hashCode(pkgFiles []*tm2.MemFile) ([]byte, error) {
	files := make([]*tm2.MemFile, len(pkgFiles))
	copy(files, pkgFiles)
	slices.SortFunc(files, func(i, j *std.MemFile) int {
		return strings.Compare(i.Name, j.Name)
	})
	hasher := sha3.New256()
	for _, file := range files {
		if _, err := hasher.Write([]byte(file.Name)); err != nil {
			return nil, errors.Wrap(err, "failed to hash file name")
		}
		if _, err := hasher.Write([]byte(file.Body)); err != nil {
			return nil, errors.Wrap(err, "failed to hash file body")
		}
	}
	return hasher.Sum(nil), nil
}
