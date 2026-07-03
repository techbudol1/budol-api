package store

import (
	"crypto/rand"
	"fmt"
	"math/big"

	"github.com/google/uuid"
)

var publicAliasAdjectives = []string{
	"Bouncy", "Brave", "Bright", "Chill", "Clever", "Cosmic", "Dancing", "Electric",
	"Fancy", "Fearless", "Jolly", "Lucky", "Mighty", "Mysterious", "Nimble", "Noisy",
	"Plucky", "Sneaky", "Sparkly", "Spicy", "Sunny", "Turbo", "Wobbly", "Zesty",
}

var publicAliasNouns = []string{
	"Adobo", "Banana", "Carabao", "Gecko", "Jeepney", "Mango", "Maya", "Otter",
	"Pancit", "Panda", "Penguin", "Rooster", "Sardine", "Squid", "Taho", "Tamaraw",
	"Tarsier", "Tilapia", "Tocino", "Turtle", "Ube", "Walis", "Yoyo", "Zebra",
}

var publicAliasRoles = []string{
	"Captain", "Detective", "Explorer", "Guardian", "Inventor", "Magician", "Mayor", "Ninja",
	"Oracle", "Pilot", "Professor", "Ranger", "Reporter", "Scholar", "Scout", "Sheriff",
	"Sidekick", "Strategist", "Traveler", "Treasurer", "Wizard", "Wrangler", "Champion", "Director",
}

// newPublicAlias creates a non-identifying public name with enough entropy that
// accidental collisions are negligible, without deriving anything from user PII.
func newPublicAlias() string {
	adjective, err := secureChoice(publicAliasAdjectives)
	if err != nil {
		return fallbackPublicAlias()
	}
	noun, err := secureChoice(publicAliasNouns)
	if err != nil {
		return fallbackPublicAlias()
	}
	role, err := secureChoice(publicAliasRoles)
	if err != nil {
		return fallbackPublicAlias()
	}
	number, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return fallbackPublicAlias()
	}
	return fmt.Sprintf("%s %s %s %06d", adjective, noun, role, number.Int64())
}

func secureChoice(values []string) (string, error) {
	index, err := rand.Int(rand.Reader, big.NewInt(int64(len(values))))
	if err != nil {
		return "", err
	}
	return values[index.Int64()], nil
}

func fallbackPublicAlias() string {
	id := uuid.New()
	return fmt.Sprintf("Mysterious Tarsier Trader %06d", (int(id[13])<<16|int(id[14])<<8|int(id[15]))%1_000_000)
}
