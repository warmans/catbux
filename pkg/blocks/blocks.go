package blocks

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"log"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/warmans/catbux/pkg/util"
)

var Genesis = &Block{Index: 0, Hash: "genesis", Timestamp: time.Time{}, Difficulty: 0, Nonce: 0}

const (
	BlockGenerationInterval      = 10
	DifficultyAdjustmentInterval = 10
)

type Block struct {
	Index      int64     `json:"index"`
	Hash       string    `json:"hash"`
	PrevHash   string    `json:"prev_hash"`
	Timestamp  time.Time `json:"timestamp"`
	Difficulty int       `json:"difficulty"`
	Nonce      int       `json:"nonce"`
	Data       string    `json:"data"`
}

func Hash(b *Block) string {
	hash := sha256.New()
	fmt.Fprintf(hash, "%d", b.Index)
	fmt.Fprintf(hash, "%s", b.PrevHash)
	fmt.Fprintf(hash, "%s", b.Timestamp.Format(time.RFC3339Nano))
	fmt.Fprintf(hash, "%d", b.Difficulty)
	fmt.Fprintf(hash, "%d", b.Nonce)
	fmt.Fprintf(hash, "%s", b.Data)
	return base64.URLEncoding.EncodeToString(hash.Sum(nil))
}

func IsValidBlock(newBlock, prevBlock *Block) error {
	// index is valid
	if expectedIndex := prevBlock.Index + 1; expectedIndex != newBlock.Index {
		return fmt.Errorf("index was wrong: expected %d got %d", expectedIndex, newBlock.Index)
	}
	// prev hash is valid
	if prevBlock.Hash != newBlock.PrevHash {
		return fmt.Errorf("preceeding hash was wrong: expected %s got %s", prevBlock.Hash, newBlock.PrevHash)
	}
	// hash is generally valid for the content
	if blockHash := Hash(newBlock); blockHash != newBlock.Hash {
		return fmt.Errorf("block hash was wrong: expected %s got %s", blockHash, newBlock.Hash)
	}
	// timestamp is more-or-less ok
	if err := isValidTimestamp(newBlock, prevBlock); err != nil {
		return err
	}
	// hash is correct for the specified difficulty
	if err := hashMatchesDifficulty(newBlock.Hash, newBlock.Difficulty); err != nil {
		return err
	}
	return nil
}

func FindNonce(block *Block) {
	start := time.Now()
	defer func() {
		log.Printf("Found block nonce %d in %0.2f seconds", block.Nonce, time.Since(start).Seconds())
	}()
	for {
		block.Hash = Hash(block)
		if err := hashMatchesDifficulty(block.Hash, block.Difficulty); err == nil {
			return
		}
		block.Nonce++
	}
}

func hashMatchesDifficulty(hash string, difficulty int) error {
	bin := util.HexToBin(hash)
	log.Println(bin)
	if len(bin) < difficulty {
		return fmt.Errorf("hash binary was not long enough (binary: %d difficulty: %d)", len(bin), difficulty)
	}
	if !strings.HasPrefix(bin, strings.Repeat("0", difficulty)) {
		return fmt.Errorf("hash did not match required difficulty (required %d zeros but hash prefix was: %s)", difficulty, bin[:difficulty-1])
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

func (c *Blockchain) Len() int64 {
	c.RLock()
	defer c.RUnlock()

	return int64(len(c.Blocks))
}

func (c *Blockchain) Replace(chain *Blockchain) error {
	return c.writeLock(func() error {
		if err := chain.IsValid(); err != nil {
			return err
		}
		if chain.GetChainDifficulty() > c.GetChainDifficulty() {
			c.Blocks = chain.Blocks
		}
		return nil
	})
}

func (c *Blockchain) GetChainDifficulty() int64 {
	total := 0.0
	for _, b := range c.Blocks {
		total += math.Pow(2, float64(b.Difficulty))
	}
	return int64(total)
}

func (c *Blockchain) GetCurrentDifficulty() int {
	lastBlock := c.Blocks[len(c.Blocks)-1]
	if lastBlock.Index%DifficultyAdjustmentInterval == 0 && lastBlock.Index != 0 {
		return c.getAdjustedDifficulty(lastBlock)
	}
	return lastBlock.Difficulty

}

func (c *Blockchain) getAdjustedDifficulty(lastBlock *Block) int {

	prevAdjustmentBlock := c.Blocks[len(c.Blocks)-DifficultyAdjustmentInterval]
	timeExpected := float64(BlockGenerationInterval * DifficultyAdjustmentInterval)
	timeTaken := lastBlock.Timestamp.Sub(prevAdjustmentBlock.Timestamp)

	if timeTaken.Seconds() < timeExpected/2 {
		return prevAdjustmentBlock.Difficulty + 1
	} else if timeTaken.Seconds() > timeExpected*2 {
		return prevAdjustmentBlock.Difficulty - 1
	}
	return prevAdjustmentBlock.Difficulty
}

func (c *Blockchain) writeLock(f func() error) error {
	c.Lock()
	defer c.Unlock()
	return f()
}

func isValidTimestamp(newBlock *Block, prevBlock *Block) error {
	if newBlock.Timestamp.Unix() < prevBlock.Timestamp.Unix()-60 {
		return fmt.Errorf(
			"block is more than 1 minute older than last block (prev: %s, new: %s)",
			prevBlock.Timestamp.Format(time.RFC3339),
			newBlock.Timestamp.Format(time.RFC3339),
		)
	}
	if now := time.Now(); newBlock.Timestamp.After(now.Add(time.Minute)) {
		return fmt.Errorf(
			"block is more than 1 minute newer than node time (block: %s, now: %s)",
			newBlock.Timestamp.Format(time.RFC3339),
			now.Format(time.RFC3339),
		)
	}
	return nil
}
