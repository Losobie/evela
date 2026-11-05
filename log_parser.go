package main

import logparser "github.com/Losobie/evela/parser"

// Local aliases keep application code concise without duplicating parser records.
// Parsing and catalog ownership live entirely in the independent parser module.
type (
	parsedLog           = logparser.Log
	damagePoint         = logparser.DamagePoint
	damageHit           = logparser.DamageHit
	damageSource        = logparser.DamageSource
	incomingSourcePoint = logparser.IncomingSourcePoint
	structureEvent      = logparser.StructureEvent
	logEvent            = logparser.Event
	ewarEvent           = logparser.EWarEvent
	logisticsEvent      = logparser.LogisticsEvent
	miningEvent         = logparser.MiningEvent
	logLocationEvent    = logparser.LocationEvent
	combatEntity        = logparser.CombatEntity
)

const (
	logTimeLayout           = logparser.TimeLayout
	spatialPhenomenaMessage = logparser.SpatialPhenomenaMessage
	miningMined             = logparser.Mined
	miningCompressed        = logparser.Compressed
	miningWaste             = logparser.Waste
)

var (
	parseLog             = logparser.Parse
	parseLogistics       = logparser.ParseLogistics
	parseIncomingEWar    = logparser.ParseIncomingEWar
	parseMining          = logparser.ParseMining
	parseLogLocation     = logparser.ParseLocation
	parseCombatEntity    = logparser.ParseCombatEntity
	incomingDamageSource = logparser.IncomingDamageSource
	loggedTarget         = logparser.LoggedTarget
	oreType              = logparser.OreType
	hullClass            = logparser.HullClass
	npcGroup             = logparser.NPCGroup
	lookupNPC            = logparser.LookupNPC
)
