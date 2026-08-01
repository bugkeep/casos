import React from "react";
import {Alert, Typography} from "antd";

const {Text} = Typography;

export default function HelmCompatibilityErrorAlert({error, onClose, t}) {
  if (!error) {return null;}
  const structured = typeof error === "object";
  return (
    <Alert
      type="error"
      message={structured ? error.message : error}
      description={structured && (error.gvk || error.detail) ? (
        <div>
          {error.gvk && (
            <div>
              <Text strong>{t("helm:Missing resource")}</Text><br />
              <Text code>{error.gvk}</Text>
            </div>
          )}
          {error.detail && (
            <div style={{marginTop: error.gvk ? 8 : 0}}>
              <Text strong>{t("helm:Compatibility details")}</Text><br />
              {error.detail}
            </div>
          )}
        </div>
      ) : undefined}
      showIcon
      style={{marginBottom: 16}}
      closable
      onClose={onClose}
    />
  );
}
