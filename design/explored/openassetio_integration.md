# Explored: OpenAssetIO Integration

OpenAssetIO (ASWF project) provides a standardized API between DCCs
(Maya, Nuke, Houdini) and asset management systems (ShotGrid, ftrack).
It answers "where is asset X?" while C4 answers "what IS this content?"

A C4 trait in OpenAssetIO could attach content identity to any asset
reference, enabling deduplication, integrity verification, and
delivery reconciliation across pipeline tools without changing
existing AMS integrations.

**Status:** Not planned for v1.0. Relevant when targeting M&E pipeline
integration beyond CLI tooling.
