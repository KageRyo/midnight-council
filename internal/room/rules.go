package room

import (
	"errors"
	"sort"
)

type Faction string

const (
	FactionCouncil Faction = "COUNCIL"
	FactionKillers Faction = "KILLERS"
)

var ErrInvalidRuleSet = errors.New("game rule set is invalid")

type NightActionRule struct {
	Type            NightActionType
	Priority        int
	AllowSelfTarget bool
}

type RoleRule struct {
	Role        Role
	Faction     Faction
	NightAction *NightActionRule
	CanShoot    bool
}

type RuleSet struct {
	roles           map[Role]RoleRule
	nightPriorities map[NightActionType]int
	deckOrder       []Role
}

func NewRuleSet(roleRules []RoleRule, deckOrder []Role) (*RuleSet, error) {
	rules := &RuleSet{
		roles:           make(map[Role]RoleRule, len(roleRules)),
		nightPriorities: make(map[NightActionType]int),
		deckOrder:       append([]Role(nil), deckOrder...),
	}
	for _, rule := range roleRules {
		if rule.Role == "" || rule.Faction == "" {
			return nil, ErrInvalidRuleSet
		}
		if _, exists := rules.roles[rule.Role]; exists {
			return nil, ErrInvalidRuleSet
		}
		if rule.NightAction != nil && (rule.NightAction.Type == "" || rule.NightAction.Priority < 0) {
			return nil, ErrInvalidRuleSet
		}
		if rule.NightAction != nil {
			nightAction := *rule.NightAction
			rule.NightAction = &nightAction
			priority, exists := rules.nightPriorities[rule.NightAction.Type]
			if exists && priority != rule.NightAction.Priority {
				return nil, ErrInvalidRuleSet
			}
			rules.nightPriorities[rule.NightAction.Type] = rule.NightAction.Priority
		}
		rules.roles[rule.Role] = rule
	}
	if _, ok := rules.roles[RoleVillager]; !ok {
		return nil, ErrInvalidRuleSet
	}
	seenDeckRoles := make(map[Role]bool, len(rules.deckOrder))
	for _, role := range rules.deckOrder {
		if role == RoleVillager {
			return nil, ErrInvalidRuleSet
		}
		if seenDeckRoles[role] {
			return nil, ErrInvalidRuleSet
		}
		if _, ok := rules.roles[role]; !ok {
			return nil, ErrInvalidRuleSet
		}
		seenDeckRoles[role] = true
	}
	return rules, nil
}

var defaultRuleSet = mustRuleSet([]RoleRule{
	{Role: RoleVillager, Faction: FactionCouncil},
	{
		Role:    RoleKiller,
		Faction: FactionKillers,
		NightAction: &NightActionRule{
			Type:     NightActionKill,
			Priority: 30,
		},
	},
	{
		Role:    RoleDetective,
		Faction: FactionCouncil,
		NightAction: &NightActionRule{
			Type:     NightActionInvestigate,
			Priority: 40,
		},
	},
	{
		Role:    RoleDoctor,
		Faction: FactionCouncil,
		NightAction: &NightActionRule{
			Type:            NightActionProtect,
			Priority:        20,
			AllowSelfTarget: true,
		},
	},
	{
		Role:    RoleEscort,
		Faction: FactionCouncil,
		NightAction: &NightActionRule{
			Type:     NightActionBlock,
			Priority: 10,
		},
	},
	{Role: RoleShooter, Faction: FactionCouncil, CanShoot: true},
}, []Role{RoleKiller, RoleDetective, RoleDoctor, RoleEscort, RoleShooter})

func mustRuleSet(roleRules []RoleRule, deckOrder []Role) *RuleSet {
	rules, err := NewRuleSet(roleRules, deckOrder)
	if err != nil {
		panic(err)
	}
	return rules
}

func DefaultRuleSet() *RuleSet {
	return defaultRuleSet
}

func (r *RuleSet) Role(role Role) (RoleRule, bool) {
	rule, ok := r.roles[role]
	if ok && rule.NightAction != nil {
		nightAction := *rule.NightAction
		rule.NightAction = &nightAction
	}
	return rule, ok
}

func (r *RuleSet) NightActionForRole(role Role) (NightActionRule, bool) {
	rule, ok := r.Role(role)
	if !ok || rule.NightAction == nil {
		return NightActionRule{}, false
	}
	return *rule.NightAction, true
}

func (r *RuleSet) CanShoot(role Role) bool {
	rule, ok := r.Role(role)
	return ok && rule.CanShoot
}

func (r *RuleSet) IsKillerAligned(role Role) bool {
	rule, ok := r.Role(role)
	return ok && rule.Faction == FactionKillers
}

func (r *RuleSet) ValidateRoleConfiguration(configuration RoleConfiguration) error {
	if configuration.Killers != 1 ||
		configuration.Detectives < 0 || configuration.Detectives > 1 ||
		configuration.Doctors < 0 || configuration.Doctors > 1 ||
		configuration.Escorts < 0 || configuration.Escorts > 1 ||
		configuration.Shooters < 0 || configuration.Shooters > 1 {
		return ErrInvalidRoleConfiguration
	}
	deckRoles := make(map[Role]bool, len(r.deckOrder))
	for _, role := range r.deckOrder {
		deckRoles[role] = true
	}
	for _, role := range []Role{RoleKiller, RoleDetective, RoleDoctor, RoleEscort, RoleShooter} {
		if configuredRoleCount(configuration, role) > 0 && !deckRoles[role] {
			return ErrInvalidRoleConfiguration
		}
	}
	return nil
}

func (r *RuleSet) RoleDeck(playerCount int, configuration RoleConfiguration) []Role {
	deck := make([]Role, 0, playerCount)
	for _, role := range r.deckOrder {
		if configuredRoleCount(configuration, role) > 0 && len(deck) < playerCount-1 {
			deck = append(deck, role)
		}
	}
	for len(deck) < playerCount {
		deck = append(deck, RoleVillager)
	}
	return deck
}

func (r *RuleSet) OrderNightActions(actions map[string]NightAction) []NightAction {
	ordered := make([]NightAction, 0, len(actions))
	for _, action := range actions {
		ordered = append(ordered, action)
	}
	sort.Slice(ordered, func(i, j int) bool {
		leftPriority := r.nightActionPriority(ordered[i].Type)
		rightPriority := r.nightActionPriority(ordered[j].Type)
		if leftPriority != rightPriority {
			return leftPriority < rightPriority
		}
		return ordered[i].PlayerID < ordered[j].PlayerID
	})
	return ordered
}

func (r *RuleSet) nightActionPriority(actionType NightActionType) int {
	if priority, ok := r.nightPriorities[actionType]; ok {
		return priority
	}
	return int(^uint(0) >> 1)
}

func configuredRoleCount(configuration RoleConfiguration, role Role) int {
	switch role {
	case RoleKiller:
		return configuration.Killers
	case RoleDetective:
		return configuration.Detectives
	case RoleDoctor:
		return configuration.Doctors
	case RoleEscort:
		return configuration.Escorts
	case RoleShooter:
		return configuration.Shooters
	default:
		return 0
	}
}
