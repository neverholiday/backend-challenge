package bcrypt

// PlaceholderHashForTest exposes the timing placeholder to the package's
// external test, which asserts its cost factor tracks the hasher's own.
const PlaceholderHashForTest = placeholderHash
