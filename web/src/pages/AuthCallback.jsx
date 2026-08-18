import React, {useEffect, useState} from "react";
import i18next from "i18next";
import * as AccountBackend from "@/backend/AccountBackend";
import * as Setting from "@/Setting";
import {Button} from "@/components/ui/button";
import {Loading} from "@/components/shared/loading";
import {ResultScreen} from "@/components/shared/misc";

function getFromLink() {
  const from = sessionStorage.getItem("from");
  sessionStorage.removeItem("from");
  return from === null ? "/" : from;
}

function AuthCallback() {
  const [msg, setMsg] = useState(null);

  useEffect(() => {
    let cancelled = false;

    // The Casdoor SDK is configured by the server, so the callback has to load
    // the sign-in options before it can exchange the authorization code.
    async function login() {
      try {
        if (!Setting.isCasdoorAvailable()) {
          const options = await AccountBackend.getSigninOptions();
          if (options.status !== "ok" || !options.data?.casdoorAvailable) {
            if (!cancelled) {
              setMsg(options.msg || i18next.t("account:Sign in is unavailable"));
            }
            return;
          }
          Setting.initCasdoorSdk(options.data.authConfig);
        }

        const res = await Setting.signin();
        if (res.status === "ok") {
          Setting.showMessage("success", i18next.t("account:Logged in successfully"));
          Setting.goToLink(getFromLink());
          return;
        }
        if (!cancelled) {
          setMsg(res.msg);
        }
      } catch (error) {
        if (!cancelled) {
          setMsg(error.message);
        }
      }
    }

    login();
    return () => {
      cancelled = true;
    };
  }, []);

  if (msg === null) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <Loading type="page" tip={i18next.t("account:Signing in...")} />
      </div>
    );
  }

  return (
    <div className="flex min-h-screen items-center justify-center">
      <ResultScreen
        status="!"
        title={i18next.t("account:Sign in failed")}
        subTitle={msg}
        extra={<Button onClick={() => Setting.goToLink("/signin")}>{i18next.t("account:Back to sign in")}</Button>}
      />
    </div>
  );
}

export default AuthCallback;
