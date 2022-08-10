import React, { useState, ReactNode, useEffect } from 'react';
import classnames from 'classnames';
import OutsideClickHandler from 'react-outside-click-handler';

import styles from './ModalWithToggle.module.scss';

function ModalWithToggle({
  toggleText,
  handleOutClick,
  headerEl,
  leftSideEl,
  rightSideEl,
  footerEl,
  noDataEl,
  modalClassName,
  modalHeight,
}: {
  toggleText: string;
  handleOutClick?: () => void;
  headerEl: string | ReactNode;
  leftSideEl: ReactNode;
  rightSideEl: ReactNode;
  footerEl?: ReactNode;
  noDataEl?: ReactNode;
  modalClassName?: string;
  modalHeight?: string;
}) {
  const [isOpen, setIsOpen] = useState(false);

  const toggleModal = () => {
    setIsOpen((v) => !v);
  };

  useEffect(() => {
    toggleModal();
  }, [toggleText]);

  const handleOutsideClick = () => {
    toggleModal();
    if (handleOutClick) handleOutClick();
  };

  return (
    <div data-testid="modal-with-toggle" className={styles.container}>
      <button
        data-testid="toggle"
        className={styles.toggle}
        onClick={toggleModal}
      >
        {toggleText}
      </button>
      {isOpen && (
        <OutsideClickHandler onOutsideClick={handleOutsideClick}>
          <div
            className={classnames(styles.modal, modalClassName)}
            data-testid="modal"
          >
            <div className={styles.modalHeader}>{headerEl}</div>
            <div className={styles.modalBody}>
              {noDataEl ? (
                noDataEl
              ) : (
                <>
                  <div
                    className={styles.side}
                    style={{ ...(modalHeight && { height: modalHeight }) }}
                  >
                    {leftSideEl}
                  </div>
                  <div
                    className={styles.side}
                    style={{ ...(modalHeight && { height: modalHeight }) }}
                  >
                    {rightSideEl}
                  </div>
                </>
              )}
            </div>
            <div className={styles.modalFooter}>{footerEl}</div>
          </div>
        </OutsideClickHandler>
      )}
    </div>
  );
}

export default ModalWithToggle;
