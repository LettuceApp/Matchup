import React from 'react';
import Button from './Button';

const ConfirmModal = ({ message, confirmLabel = 'Confirm', onConfirm, onCancel, danger = false }) => (
  <div className="edit-profile-overlay" onClick={onCancel}>
    <div className="edit-profile-modal" onClick={(e) => e.stopPropagation()}>
      <p style={{ color: 'rgba(226,232,240,0.85)', fontSize: '0.95rem', margin: 0 }}>{message}</p>
      <div className="edit-profile-actions">
        <Button className="profile-secondary-button" onClick={onCancel}>
          Cancel
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
