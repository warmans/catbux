package blocks

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/pkg/errors"
	"github.com/warmans/catbux/pkg/crypto"
)

type TxnInSet struct {
	mu    sync.RWMutex
	set   []*TxnIn
	index map[string]struct{}
}

func (s *TxnInSet) Append(txn *TxnIn) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, found := s.index[txn.TxnOutID]; found {
		return
	}
	s.set = append(s.set, txn)
}

func (s *TxnInSet) Len() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return int64(len(s.set))
}

func (s *TxnInSet) TotalValue(txn *Transaction, unspent []*TxnOutUnspent) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	totalTxnInValue := int64(0)
	for _, in := range s.set {
		if err := in.Validate(txn, unspent); err != nil {
			return 0, errors.Wrapf(err, "txn id %s contained invalid txn in data", txn.ID)
		}
		amnt, err := getTxnInAmount(in, unspent)
		if err != nil {
			return 0, err
		}
		totalTxnInValue += amnt
	}
	return totalTxnInValue, nil
}

func (s *TxnInSet) WriteToHash(hash io.Writer) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, in := range s.set {
		fmt.Fprintf(hash, "%s%d", in.TxnOutID, in.TxnOutIndex)
	}
}

func (s *TxnInSet) Get(index int64) (*TxnIn, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if index >= s.Len() {
		return nil, fmt.Errorf("invalid IN TXN index: %d", index)
	}
	return s.set[index], nil
}

func (s *TxnInSet) MarshalJSON() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return json.Marshal(s.set)
}

func (s *TxnInSet) Spent() []*TxnOutSpent {
	c := make([]*TxnOutSpent, len(s.set))
	for k, in := range s.set {
		c[k] = &TxnOutSpent{TxnOutID: in.TxnOutID, TxnOutIndex: in.TxnOutIndex}
	}
	return c
}

type TxnIn struct {
	TxnOutID    string `json:"txn_out_id"`
	TxnOutIndex int64  `json:"txn_out_index"`
	Signature   string `json:"signature"`
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
	Address string `json:"address"`
	Amount  int64  `json:"amount"`
}

type TxnOutUnspent struct {
	TxnOutID    string `json:"txn_out_id"`
	TxnOutIndex int64  `json:"txn_out_index"`
	Address     string `json:"address"`
	Amount      int64  `json:"amount"`
}

type TxnOutSpent struct {
	TxnOutID    string `json:"txn_out_id"`
	TxnOutIndex int64  `json:"txn_out_index"`
}

type Transaction struct {
	ID     string    `json:"id"`
	TxnIn  TxnInSet  `json:"txn_in"`
	TxnOut []*TxnOut `json:"txn_out"`
}

func (t *Transaction) GetTxnIn(index int64) (*TxnIn, error) {
	return t.TxnIn.Get(index)
}

func (t *Transaction) Validate(unspent []*TxnOutUnspent) error {
	if t.ID != GetTransactionID(t) {
		return fmt.Errorf("invalid transaction ID")
	}

	totalTxnOutValue := int64(0)
	for _, out := range t.TxnOut {
		totalTxnOutValue += out.Amount
	}

	totalTxnInValue, err := t.TxnIn.TotalValue(t, unspent)
	if err != nil {
		return err
	}

	if totalTxnInValue != totalTxnOutValue {
		return fmt.Errorf("failed to reconsile in/out values (total in: %d total out:%d", totalTxnInValue, totalTxnOutValue)
	}
	return nil
}

func GetTransactionID(t *Transaction) string {
	hash := sha256.New()

	t.TxnIn.WriteToHash(hash)

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

func ProcessTransactions(txns []*Transaction, unspent []*TxnOutUnspent, blockIndex int64) error {
	if err := ValidateBlockTransactions(txns, unspent, blockIndex); err != nil {
		return err
	}

	return nil
}

func ValidateBlockTransactions(txns []*Transaction, unspent []*TxnOutUnspent, blockIndex int64) error {

	//todo: coninbase txn

	//check for duplication in txnIn records
	if err := validateTxnInSets(txns); err != nil {
		return err
	}

	for _, txn := range txns {
		if err := txn.Validate(unspent); err != nil {
			return err
		}
	}

	return nil
}

func UpdateUnspentTxns(newTxns []*Transaction, unspent []*TxnOutUnspent) []*TxnOutUnspent {

	spent := make([]*TxnOutSpent, 0)
	for _, txn := range newTxns {
		spent = append(spent, txn.TxnIn.Spent()...)
	}
	newUnspent := make([]*TxnOutUnspent, 0)

	//filter now spent rows
	for _, u := range unspent {
		if !isSpent(spent, u.TxnOutID, u.TxnOutIndex) {
			newUnspent = append(newUnspent, u)
		}
	}
	//add new unspent rows
	for _, txn := range newTxns {
		for i, out := range txn.TxnOut {
			newUnspent = append(newUnspent, &TxnOutUnspent{TxnOutID: txn.ID, TxnOutIndex: int64(i), Address: out.Address, Amount: out.Amount})
		}
	}
	return newUnspent
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

func validateTxnInSets(txns []*Transaction) error {
	index := make(map[string]struct{})
	for _, t := range txns {
		for _, sp := range t.TxnIn.Spent() {
			if _, found := index[sp.TxnOutID]; found {
				return fmt.Errorf("same txn out id was found in two different txn in records: %s", sp.TxnOutID)
			}
			index[sp.TxnOutID] = struct{}{}
		}
	}
	return nil
}

func isSpent(unspent []*TxnOutSpent, outId string, outIndex int64) bool {
	for _, u := range unspent {
		if u.TxnOutID == outId && u.TxnOutIndex == outIndex {
			return true
		}
	}
	return false
}
