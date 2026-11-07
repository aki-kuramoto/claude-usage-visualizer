package usage

// The reset times name their time zone ("Asia/Tokyo"). Windows has no time
// zone database that Go can read; without this, Go would look for the one in
// its own installation, which a machine running the built executable may not
// have. This adds about 450 KB to the executable.
import _ "time/tzdata"
