// Package parser reads English EVE Online game logs without a UI, database,
// network connection or C compiler. Parse accepts UTF-8 and BOM-marked UTF-16.
// Times are UTC; event Second fields are Unix timestamps at one-second resolution.
//
// Each call processes one log. A missing Listener header leaves Character empty;
// callers decide whether to retain unassigned logs. Damage sources are interned
// per log: an incoming hit's Source indexes Log.Sources; outgoing hits use -1.
// Hits includes misses, while Points and IncomingSources count landed hits.
// Treat returned records as immutable when sharing them with other goroutines.
//
// Ship/location history, cross-file deduplication, filters, rates and engagement
// detection belong to the consuming application. Embedded CCP catalogs retain
// their separate terms; see NOTICE and LICENSE.
package parser
