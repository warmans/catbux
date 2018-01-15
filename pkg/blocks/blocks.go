package blocks

import (
	"time"
	"crypto/sha256"
	"fmt"
	"encoding/base64"
	"log"
	"sync"
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

func NewBlockchain(peers *Peers) *Blockchain {
	return &Blockchain{chain: []*Block{Genesis}, peers: peers}
}

type Blockchain struct {
	chain     []*Block
	peers     *Peers
	sync.RWMutex
}

func (c *Blockchain) Last() *Block {
	c.RLock()
	defer c.RUnlock()
	return c.chain[len(c.chain)-1]
}

func (c *Blockchain) Append(data string) (error) {
	newBlock := &Block{
		Index:     int64(len(c.chain)),
		PrevHash:  c.Last().Hash,
		Timestamp: time.Now(),
		Data:      data,
	}
	newBlock.Hash = Hash(newBlock)

	c.writeLock(func() {
		if err := IsValidBlock(newBlock, c.chain[len(c.chain)-1]); err != nil {
			log.Printf("failed to add block: %s", err.Error())
			return
		}
		c.chain = append(c.chain, newBlock)
	})
	c.peers.Broadcast(newBlock)
	return nil
}

func (c *Blockchain) IsValid() error {
	c.RLock()
	defer c.RUnlock()

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

func (c *Blockchain) Snapshot() []Block {
	c.RLock()
	defer c.RUnlock()

	snapshot := make([]Block, len(c.chain))
	for k, b := range c.chain {
		snapshot[k] = *b
	}
	return snapshot
}

func (c *Blockchain) Len() int {
	c.RLock()
	defer c.RUnlock()

	return len(c.chain)
}

func (c *Blockchain) writeLock(f func()) {
	c.Lock()
	defer c.Unlock()
	f()
}

func (c *Blockchain) Replace(chain *Blockchain) error {
	if err := chain.IsValid(); err != nil {
		return err
	}
	if c.Len() < chain.Len() {
		c.chain = chain.chain
	}
	return nil
}
