// AdminModal / AdminModalActions moved to the generic components/ui/Modal
// (S2) so admin forms and page-level overlays share one dialog
// implementation. Kept as a re-export under the old names so every existing
// importer (AdminOrgsPage, AdminClustersPage, AdminTokensPage, TeamsPage,
// GitPage, AdminUsersPage, AdminAuthPage) needs no change.
export { Modal as AdminModal, ModalActions as AdminModalActions } from '@/components/ui/Modal';
