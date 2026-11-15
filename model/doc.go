// Package model is the pure-domain layer of go-git-platform: the shared
// entity types and value vocabularies every layer above speaks in —
// repositories, change requests, issues, labels, milestones, reviews,
// webhooks, search results, notifications, gists, releases, CI job logs,
// migrations, commit statuses and deployments — plus the Platform
// enumeration and its seven platform constants.
//
// # Layering
//
// model is a leaf: it imports nothing from this repository (standard
// library only). provider (the contract seam), runtime (the machines),
// adapter (the platform implementations), contracttest and downstream
// consumers all reference these types as model.Xxx; nothing here may
// import any of them (spec §4.1 edge table; machine guard in S6).
// Parameter/option structs (XxxOptions) and the
// capability interfaces stay in provider — model holds only the entities
// platforms return and events carry.
package model
