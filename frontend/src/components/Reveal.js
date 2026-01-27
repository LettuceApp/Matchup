import React, { forwardRef, useRef } from 'react';
import { motion, useInView } from 'framer-motion';

const Reveal = forwardRef(
  ({ as = 'div', className = '', delay = 0, children, ...rest }, ref) => {
    const localRef = useRef(null);
    const isInView = useInView(localRef, { once: true, margin: '-60px 0px' });

    const MotionTag = motion[as] || motion.div;

    const setRefs = (node) => {
      localRef.current = node;
      if (typeof ref === 'function') {
        ref(node);
      } else if (ref) {
        ref.current = node;
      }
    };

    return (
      <MotionTag
        ref={setRefs}
        className={className}
        initial={{ opacity: 0, y: 18 }}
        animate={isInView ? { opacity: 1, y: 0 } : undefined}
        transition={{ duration: 0.4, ease: 'easeOut', delay }}
        {...rest}
      >
        {children}
      </MotionTag>
    );
  }
);

Reveal.displayName = 'Reveal';

export default Reveal;
