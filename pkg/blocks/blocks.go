package blocks

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"sync"
	"time"
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
	return base64.URLEncoding.EncodeToString(hash.Sum(nil))
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
	return &Blockchain{Blocks: []*Block{Genesis}}
}

type Blockchain struct {
	Blocks []*Block `json:"blocks"`
	sync.RWMutex
}

func (c *Blockchain) Last() *Block {
	c.RLock()
	defer c.RUnlock()
	return c.Blocks[len(c.Blocks)-1]
}

func (c *Blockchain) NewBlock(data string) (*Block, error) {
	newBlock := &Block{
		Index:     int64(len(c.Blocks)),
		PrevHash:  c.Last().Hash,
		Timestamp: time.Now(),
		Data:      data,
	}
	newBlock.Hash = Hash(newBlock)

	err := c.writeLock(func() error {
		if err := IsValidBlock(newBlock, c.Blocks[len(c.Blocks)-1]); err != nil {
			return err
		}
		c.Blocks = append(c.Blocks, newBlock)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return newBlock, nil
}

func (c *Blockchain) Append(block *Block) error {
	err := c.writeLock(func() error {
		if err := IsValidBlock(block, c.Blocks[len(c.Blocks)-1]); err != nil {
			return err
		}
		c.Blocks = append(c.Blocks, block)
		return nil
	})
	return err
}

func (c *Blockchain) IsValid() error {
	c.RLock()
	defer c.RUnlock()

	if len(c.Blocks) == 0 {
		return fmt.Errorf("genesis block was missing")
	}
	if c.Blocks[0].Hash != Genesis.Hash {
		return fmt.Errorf("genesis block was unexpected: %+v", Genesis)
	}
	for k := 1; k < len(c.Blocks); k++ {
		if err := IsValidBlock(c.Blocks[k], c.Blocks[k-1]); err != nil {
			return err
		}
	}
	return nil
}

func (c *Blockchain) Snapshot() *Blockchain {
	c.RLock()
	defer c.RUnlock()

	snapshot := &Blockchain{Blocks: make([]*Block, len(c.Blocks))}
	for k, b := range c.Blocks {
		deref := *b
		snapshot.Blocks[k] = &deref
	}
	return snapshot
}

func (c *Blockchain) Len() int {
	c.RLock()
	defer c.RUnlock()

	return len(c.Blocks)
}

func (c *Blockchain) Replace(chain *Blockchain) error {
	if err := chain.IsValid(); err != nil {
		return err
	}
	if c.Len() < chain.Len() {
		c.Blocks = chain.Blocks
	}
	return nil
}

func (c *Blockchain) writeLock(f func() error) error {
	c.Lock()
	defer c.Unlock()
	return f()
}
