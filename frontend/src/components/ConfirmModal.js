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
      {title && (
        <h2 style={{ color: 'rgba(226,232,240,0.95)', fontSize: '1.1rem', margin: '0 0 0.5rem' }}>
          {title}
        </h2>
      )}
      <p style={{ color: 'rgba(226,232,240,0.85)', fontSize: '0.95rem', margin: 0 }}>{message}</p>
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
