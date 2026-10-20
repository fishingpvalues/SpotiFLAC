package backend

// GetAmazonMusicAPIBaseURL returns the base URL used for the Amazon status
// check. The former dedicated host (amazon.spotbye.qzz.io) no longer resolves,
// so this follows the Amazon community endpoint, which honours
// SPOTIFLAC_AMAZON_COMMUNITY_URL.
func GetAmazonMusicAPIBaseURL() string {
	return amazonCommunityBaseURL()
}
