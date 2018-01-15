package blocks

import (
	"time"
	"crypto/sha256"
	"fmt"
)

var Genesis = &Block{Index: 0, Hash: "genesis", Timestamp: time.Time{}}

type Block struct {
	Index     int64     `json:"index"`
	Hash      string    `json:"hash"`
	PrevHash  string    `json:"prev_hash"`
	Timestamp time.Time `json:"timestamp"`
	Data      string    `json:"data"`
}

func Hash(b *Block) string {
	hash := sha256.New()
	fmt.Fprintf(hash, "%d", b.Index)
	fmt.Fprintf(hash, "%s", b.PrevHash)
	fmt.Fprintf(hash, "%s", b.Timestamp.Format(time.RFC3339Nano))
	fmt.Fprintf(hash, "%s", b.Data)
	return string(hash.Sum([]byte(""))[:])
}

func IsValidBlock(newBlock, prevBlock *Block) error {
	if expectedIndex := prevBlock.Index + 1; expectedIndex != newBlock.Index {
		return fmt.Errorf("index was wrong: expected %d got %d", expectedIndex, newBlock.Index)
	}
	if prevBlock.Hash != newBlock.PrevHash {
		return fmt.Errorf("preceeding hash was wrong: expected %s got %s", prevBlock.Hash, newBlock.PrevHash)
	}
	if blockHash := Hash(newBlock); blockHash != newBlock.Hash {
		return fmt.Errorf("block hash was wrong: expected %s got %s", blockHash, newBlock.Hash)
	}
	return nil
}

func NewBlockchain() *Blockchain {
	return &Blockchain{chain: []*Block{Genesis}}
}

type Blockchain struct {
	chain []*Block `json:"chain"`
}

func (c *Blockchain) Last() *Block {
	return c.chain[len(c.chain)-1]
}

func (c *Blockchain) Append(data string) {
	newBlock := &Block{
		Index:     int64(len(c.chain)),
		PrevHash:  c.Last().Hash,
		Timestamp: time.Now(),
		Data:      data,
	}
	newBlock.Hash = Hash(newBlock)

	c.chain = append(c.chain, newBlock)
}

func (c *Blockchain) IsValid() error {
	if len(c.chain) == 0 {
		return fmt.Errorf("genesis block was missing")
	}
	if c.chain[0] != Genesis {
		return fmt.Errorf("genesis block was unexpected: %+v", Genesis)
	}
	for k := 1; k < len(c.chain); k++ {
		if err := IsValidBlock(c.chain[k], c.chain[k-1]); err != nil {
			return err
		}
	}
	return nil
}

func (c *Blockchain) Len() int {
	return len(c.chain)
}

func (c *Blockchain) Replace(chain *Blockchain) error {
	if err := chain.IsValid(); err != nil {
		return err
	}
	if c.Len() < chain.Len() {
		c.chain = chain.chain
	}

	//todo: broadcast new chain
	return nil
}
