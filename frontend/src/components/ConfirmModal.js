import React from 'react';
import Button from './Button';
import '../styles/ConfirmModal.css';

const ConfirmModal = ({
  title,
  message,
  confirmLabel = 'Confirm',
  cancelLabel = 'Cancel',
  onConfirm,
  onCancel,
  danger = false,
}) => (
  <div className="edit-profile-overlay" onClick={onCancel}>
    <div className="edit-profile-modal" onClick={(e) => e.stopPropagation()}>
      {/*
        Title + message used to ship inline `rgba(226,232,240,…)` slate
        colors, baked for dark-mode only. On light mode the modal
        surface is white and slate-200 text vanished into it. Pulled
        the colors out into the stylesheet (.confirm-modal__title /
        .confirm-modal__message) so they consume the same
        `--text-primary` / `--text-secondary` tokens every other
        theme-aware surface uses — readable in both modes.
      */}
      {title && <h2 className="confirm-modal__title">{title}</h2>}
      <p className="confirm-modal__message">{message}</p>
      <div className="edit-profile-actions">
        <Button className="profile-secondary-button" onClick={onCancel}>
          {cancelLabel}
        </Button>
        <Button
          className={danger ? 'matchup-danger-button' : 'profile-primary-button'}
          onClick={onConfirm}
        >
          {confirmLabel}
        </Button>
      </div>
    </div>
  </div>
);

export default ConfirmModal;
