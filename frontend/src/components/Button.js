import React from 'react';
import { motion } from 'framer-motion';
import PropTypes from 'prop-types';

// Use default parameters instead of defaultProps
const Button = ({
  onClick = () => {},
  type = "button",
  children,
  className = '',
  disabled = false,
  ...rest
}) => {
  return (
    <motion.button
      type={type}
      onClick={onClick}
      className={className}
      disabled={disabled}
      whileHover={disabled ? undefined : { y: -1 }}
      whileTap={disabled ? undefined : { scale: 0.98 }}
      {...rest}
    >
      {children}
    </motion.button>
  );
};

Button.propTypes = {
  onClick: PropTypes.func,
  children: PropTypes.node.isRequired,
  className: PropTypes.string,
  type: PropTypes.oneOf(['button', 'submit', 'reset']),
  disabled: PropTypes.bool,
};

export default Button;
