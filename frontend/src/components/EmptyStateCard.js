import React from 'react';
import { FiStar } from 'react-icons/fi';
import Button from './Button';
import '../styles/EmptyStateCard.css';

const EmptyStateCard = ({
  title,
  description,
  ctaLabel,
  onCta,
  suggestions = [],
  icon,
  tips = [],
}) => {
  const IconComponent = typeof icon === 'function' ? icon : null;
  const renderIcon = () => {
    if (React.isValidElement(icon)) return icon;
    if (IconComponent) return <IconComponent />;
    if (typeof icon === 'string') return <span>{icon}</span>;
    return <FiStar />;
  };

  return (
    <div className="empty-state-card">
      <div className="empty-state-icon" aria-hidden="true">
        {renderIcon()}
      </div>
      <h3>{title}</h3>
      <p>{description}</p>
      {tips.length > 0 && (
        <div className="empty-state-tips">
          {tips.map((tip) => (
            <span key={tip} className="empty-state-tip">
              {tip}
            </span>
          ))}
        </div>
      )}
      {suggestions.length > 0 && (
        <div className="empty-state-suggestions">
          {suggestions.map((suggestion) => (
            <button
              key={suggestion}
              type="button"
              className="empty-state-chip"
            >
              {suggestion}
            </button>
          ))}
        </div>
      )}
      {ctaLabel && (
        <Button onClick={onCta} className="empty-state-cta">
          {ctaLabel}
        </Button>
      )}
    </div>
  );
};

export default EmptyStateCard;
