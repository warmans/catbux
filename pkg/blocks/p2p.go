package blocks

import (
	"log"
	"github.com/hashicorp/memberlist"
)

func NewPeers(config *memberlist.Config, seedNodes ...string) (*Peers, error){

	list, err := memberlist.Create(config)
	if err != nil {
		return nil, err
	}
	if len(seedNodes) > 0 {
		if _, err := list.Join(seedNodes); err != nil {
			return nil, err
		}
	}
	return &Peers{members: list}, nil
}

type Peers struct {
	members *memberlist.Memberlist
}

func (p *Peers) Broadcast(block *Block) {
	log.Printf("broadcasting new block to peers\n")

	for _, member := range p.members.Members() {
		//todo: send the block
		log.Printf("==> member: %s %s\n", member.Name, member.Addr)
	}
}
