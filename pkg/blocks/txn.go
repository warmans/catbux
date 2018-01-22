package blocks

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/base64"
	"fmt"

	"github.com/pkg/errors"
	"github.com/warmans/catbux/pkg/crypto"
)

type TxnIn struct {
	TxnOutID    string
	TxnOutIndex int64
	Signature   string
}

func (t *TxnIn) Validate(txn *Transaction, unspent []*TxnOutUnspent) error {
	found := findUnspentTxnOut(t.TxnOutID, t.TxnOutIndex, unspent)
	if found == nil {
		return fmt.Errorf("unspent txn out not found")
	}
	pubKey, err := crypto.PubKeyFromBase64(found.Address)
	if err != nil {
		return err
	}
	if !crypto.Verify([]byte(txn.ID), []byte(t.Signature), pubKey) {
		return fmt.Errorf("failed to verify transaction ID against signature/key")
	}
	return nil
}

type TxnOut struct {
	Address string
	Amount  int64
}

type TxnOutUnspent struct {
	TxnOutID    string
	TxnOutIndex int64
	Address     string
	Amount      int64
}

type Transaction struct {
	ID     string
	TxnIn  []*TxnIn
	TxnOut []*TxnOut
}

func (t *Transaction) GetTxnIn(index int64) (*TxnIn, error) {
	if index >= int64(len(t.TxnIn)) {
		return nil, fmt.Errorf("invalid IN TXN index: %d", index)
	}
	return t.TxnIn[index], nil
}

func (t *Transaction) Validate(unspent []*TxnOutUnspent) error {
	if t.ID != GetTransactionID(t) {
		return fmt.Errorf("invalid transaction ID")
	}

	totalTxnInValue := int64(0)
	for _, in := range t.TxnIn {
		if err := in.Validate(t, unspent); err != nil {
			return errors.Wrapf(err, "txn id %s contained invalid txn in data", t.ID)
		}
		amnt, err := getTxnInAmount(in, unspent)
		if err != nil {
			return err
		}
		totalTxnInValue += amnt
	}

	totalTxnOutValue := int64(0)
	for _, out := range t.TxnOut {
		totalTxnOutValue += out.Amount
	}

	if totalTxnInValue != totalTxnOutValue {
		return fmt.Errorf("failed to reconsile in/out values (total in: %d total out:%d", totalTxnInValue, totalTxnOutValue)
	}
	return nil
}

func GetTransactionID(t *Transaction) string {
	hash := sha256.New()
	for _, in := range t.TxnIn {
		fmt.Fprintf(hash, "%s%d", in.TxnOutID, in.TxnOutIndex)
	}
	for _, out := range t.TxnOut {
		fmt.Fprintf(hash, "%s%d", out.Address, out.Amount)

	}
	return base64.URLEncoding.EncodeToString(hash.Sum(nil))
}

func SignTxnIn(txn *Transaction, txnInIndex int64, key *ecdsa.PrivateKey, unspent []*TxnOutUnspent) (string, error) {
	txnIn, err := txn.GetTxnIn(txnInIndex)
	if err != nil {
		return "", err
	}
	txnOutUnspentRef := findUnspentTxnOut(txnIn.TxnOutID, txnIn.TxnOutIndex, unspent)
	if txnOutUnspentRef == nil {
		return "", fmt.Errorf("failed to find referenced unspent txn")
	}

	signature, err := crypto.Sign([]byte(txn.ID), key)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(signature), nil

}

func findUnspentTxnOut(txnOutID string, txnOutIndex int64, unspent []*TxnOutUnspent) *TxnOutUnspent {
	for _, u := range unspent {
		if u.TxnOutID == txnOutID && u.TxnOutIndex == txnOutIndex {
			return u
		}
	}
	return nil
}

func getTxnInAmount(txnIn *TxnIn, unspent []*TxnOutUnspent) (int64, error) {
	rec := findUnspentTxnOut(txnIn.TxnOutID, txnIn.TxnOutIndex, unspent)
	if rec == nil {
		return 0, fmt.Errorf("failed to locate referenced unspent txn out")
	}
	return rec.Amount, nil
}
