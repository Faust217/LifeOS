package identity

// OwnerID identifies the LifeOS user whose data a request may access. Only
// this package can set its value; the auth guards do so after checking a session.
// Future services should accept OwnerID, not a client-provided owner string.
type OwnerID struct{ value string }

// String supplies the ID for owner-scoped SQL parameters. A zero OwnerID must
// fail closed in a repository; it is never a valid owner.
func (id OwnerID) String() string { return id.value }

// Valid reports whether this value came from a verified nonempty user ID.
func (id OwnerID) Valid() bool { return id.value != "" }
