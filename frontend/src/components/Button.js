import React from 'react';
import PropTypes from 'prop-types';

// Use default parameters instead of defaultProps
const Button = ({ onClick, type = "button", children, className = '' }) => {
  return (
    <button type={type} onClick={onClick} className={className}>
      {children}
    </button>
  );
};

Button.propTypes = {
  onClick: PropTypes.func.isRequired,
  children: PropTypes.node.isRequired,
  className: PropTypes.string,
};

export default Button;