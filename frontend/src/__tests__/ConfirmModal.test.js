import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import ConfirmModal from '../components/ConfirmModal';

describe('ConfirmModal', () => {
  it('renders the message', () => {
    render(
      <ConfirmModal
        message="Are you sure?"
        onConfirm={() => {}}
        onCancel={() => {}}
      />
    );
    expect(screen.getByText('Are you sure?')).toBeInTheDocument();
  });

  it('renders the default confirm label', () => {
    render(
      <ConfirmModal
        message="Delete?"
        onConfirm={() => {}}
        onCancel={() => {}}
      />
    );
    expect(screen.getByText('Confirm')).toBeInTheDocument();
  });

  it('renders a custom confirm label', () => {
    render(
      <ConfirmModal
        message="Delete?"
        confirmLabel="Remove"
        onConfirm={() => {}}
        onCancel={() => {}}
      />
    );
    expect(screen.getByText('Remove')).toBeInTheDocument();
  });

  it('calls onConfirm when confirm button is clicked', () => {
    const onConfirm = jest.fn();
    render(
      <ConfirmModal
        message="Delete?"
        onConfirm={onConfirm}
        onCancel={() => {}}
      />
    );
    fireEvent.click(screen.getByText('Confirm'));
    expect(onConfirm).toHaveBeenCalledTimes(1);
  });

  it('calls onCancel when cancel button is clicked', () => {
    const onCancel = jest.fn();
    render(
      <ConfirmModal
        message="Delete?"
        onConfirm={() => {}}
        onCancel={onCancel}
      />
    );
    fireEvent.click(screen.getByText('Cancel'));
    expect(onCancel).toHaveBeenCalledTimes(1);
  });

  it('applies danger class when danger=true', () => {
    render(
      <ConfirmModal
        message="Delete?"
        danger={true}
        onConfirm={() => {}}
        onCancel={() => {}}
      />
    );
    expect(screen.getByText('Confirm')).toHaveClass('matchup-danger-button');
  });

  it('applies primary class when danger=false', () => {
    render(
      <ConfirmModal
        message="Delete?"
        danger={false}
        onConfirm={() => {}}
        onCancel={() => {}}
      />
    );
    expect(screen.getByText('Confirm')).toHaveClass('profile-primary-button');
  });
});
