package cdn

// ProvisionedSpaceID is the canonical ULID of the mounted public CDN Space on
// production. It always refers to the public destination Space that clients
// auto-mount, never to the private authoring Space. Dev builds may override
// =SpaceID()= via =SPACEWAVE_CDN_SPACE_ID= to point at the staging public CDN
// Space =01kpfs6hyxeamz1a5hwwqph291= (or any other test public Space).
const ProvisionedSpaceID = "01kpn3x0y79yr94ps1yae206vp"
